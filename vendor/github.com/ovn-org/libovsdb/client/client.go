package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/cenkalti/rpc2"
	"github.com/cenkalti/rpc2/jsonrpc"
	"github.com/ovn-org/libovsdb/cache"
	"github.com/ovn-org/libovsdb/model"
	"github.com/ovn-org/libovsdb/ovsdb"
)

// OvsdbClient is an OVSDB client
type OvsdbClient struct {
	rpcClient     *rpc2.Client
	Schema        ovsdb.DatabaseSchema
	handlers      []ovsdb.NotificationHandler
	handlersMutex *sync.Mutex
	Cache         *cache.TableCache
	stopCh        chan struct{}
	api           API
}

func newOvsdbClient() *OvsdbClient {
	// Cache initialization is delayed because we first need to obtain the schema
	ovs := &OvsdbClient{
		handlersMutex: &sync.Mutex{},
		stopCh:        make(chan struct{}),
	}
	return ovs
}

// Constants defined for libovsdb
const (
	SSL  = "ssl"
	TCP  = "tcp"
	UNIX = "unix"
)

// Connect to an OVSDB Server using the provided endpoint in OVSDB Connection Format
// For more details, see the ovsdb(7) man page
// The connection can be configured using one or more Option(s), like WithTLSConfig
// If no WithEndpoint option is supplied, the default of unix:/var/run/openvswitch/ovsdb.sock is used
func Connect(ctx context.Context, database *model.DBModel, opts ...Option) (*OvsdbClient, error) {
	var c net.Conn
	var dialer net.Dialer
	var err error
	var u *url.URL

	options, err := newOptions(opts...)
	if err != nil {
		return nil, err
	}

	for _, endpoint := range options.endpoints {
		if u, err = url.Parse(endpoint); err != nil {
			return nil, err
		}
		switch u.Scheme {
		case UNIX:
			c, err = dialer.DialContext(ctx, u.Scheme, u.Path)
		case TCP:
			c, err = dialer.DialContext(ctx, u.Scheme, u.Opaque)
		case SSL:
			dialer := tls.Dialer{
				Config: options.tlsConfig,
			}
			c, err = dialer.DialContext(ctx, "tcp", u.Opaque)
		default:
			err = fmt.Errorf("unknown network protocol %s", u.Scheme)
		}
		if err == nil {
			return newRPC2Client(c, database)
		}
	}
	return nil, fmt.Errorf("failed to connect to endpoints %q: %v", options.endpoints, err)
}

func newRPC2Client(conn net.Conn, database *model.DBModel) (*OvsdbClient, error) {
	ovs := newOvsdbClient()
	ovs.rpcClient = rpc2.NewClientWithCodec(jsonrpc.NewJSONCodec(conn))
	ovs.rpcClient.SetBlocking(true)
	ovs.rpcClient.Handle("echo", func(_ *rpc2.Client, args []interface{}, reply *[]interface{}) error {
		return ovs.echo(args, reply)
	})
	ovs.rpcClient.Handle("update", func(_ *rpc2.Client, args []json.RawMessage, reply *[]interface{}) error {
		return ovs.update(args, reply)
	})
	go ovs.rpcClient.Run()
	go ovs.handleDisconnectNotification()

	dbs, err := ovs.ListDbs()
	if err != nil {
		ovs.rpcClient.Close()
		return nil, err
	}

	found := false
	for _, db := range dbs {
		if db == database.Name() {
			found = true
			break
		}
	}
	if !found {
		ovs.rpcClient.Close()
		return nil, fmt.Errorf("target database not found")
	}

	schema, err := ovs.GetSchema(database.Name())
	errors := database.Validate(schema)
	if len(errors) > 0 {
		var combined []string
		for _, err := range errors {
			combined = append(combined, err.Error())
		}
		return nil, fmt.Errorf("database validation error (%d): %s", len(errors),
			strings.Join(combined, ". "))
	}

	if err == nil {
		ovs.Schema = *schema
		if cache, err := cache.NewTableCache(schema, database, nil); err == nil {
			ovs.Cache = cache
			ovs.Register(ovs.Cache)
			ovs.api = newAPI(ovs.Cache)
		} else {
			ovs.rpcClient.Close()
			return nil, err
		}
	} else {
		ovs.rpcClient.Close()
		return nil, err
	}

	go ovs.Cache.Run(ovs.stopCh)

	return ovs, nil
}

// Register registers the supplied NotificationHandler to receive OVSDB Notifications
func (ovs *OvsdbClient) Register(handler ovsdb.NotificationHandler) {
	ovs.handlersMutex.Lock()
	defer ovs.handlersMutex.Unlock()
	ovs.handlers = append(ovs.handlers, handler)
}

//Get Handler by index
func getHandlerIndex(handler ovsdb.NotificationHandler, handlers []ovsdb.NotificationHandler) (int, error) {
	for i, h := range handlers {
		if reflect.DeepEqual(h, handler) {
			return i, nil
		}
	}
	return -1, fmt.Errorf("handler not found")
}

// Unregister the supplied NotificationHandler to not receive OVSDB Notifications anymore
func (ovs *OvsdbClient) Unregister(handler ovsdb.NotificationHandler) error {
	ovs.handlersMutex.Lock()
	defer ovs.handlersMutex.Unlock()
	i, err := getHandlerIndex(handler, ovs.handlers)
	if err != nil {
		return err
	}
	ovs.handlers = append(ovs.handlers[:i], ovs.handlers[i+1:]...)
	return nil
}

// RFC 7047 : Section 4.1.6 : Echo
func (ovs *OvsdbClient) echo(args []interface{}, reply *[]interface{}) error {
	*reply = args
	ovs.handlersMutex.Lock()
	defer ovs.handlersMutex.Unlock()
	for _, handler := range ovs.handlers {
		handler.Echo(nil)
	}
	return nil
}

// RFC 7047 : Update Notification Section 4.1.6
func (ovs *OvsdbClient) update(args []json.RawMessage, reply *[]interface{}) error {
	var value string
	if len(args) > 2 {
		return fmt.Errorf("update requires exactly 2 args")
	}
	err := json.Unmarshal(args[0], &value)
	if err != nil {
		return err
	}
	var updates ovsdb.TableUpdates
	err = json.Unmarshal(args[1], &updates)
	if err != nil {
		return err
	}
	// Update the local DB cache with the tableUpdates
	ovs.handlersMutex.Lock()
	defer ovs.handlersMutex.Unlock()
	for _, handler := range ovs.handlers {
		handler.Update(value, updates)
	}
	*reply = []interface{}{}
	return nil
}

// GetSchema returns the schema in use for the provided database name
// RFC 7047 : get_schema
func (ovs OvsdbClient) GetSchema(dbName string) (*ovsdb.DatabaseSchema, error) {
	args := ovsdb.NewGetSchemaArgs(dbName)
	var reply ovsdb.DatabaseSchema
	err := ovs.rpcClient.Call("get_schema", args, &reply)
	if err != nil {
		return nil, err
	}
	ovs.Schema = reply
	return &reply, err
}

// ListDbs returns the list of databases on the server
// RFC 7047 : list_dbs
func (ovs OvsdbClient) ListDbs() ([]string, error) {
	var dbs []string
	err := ovs.rpcClient.Call("list_dbs", nil, &dbs)
	if err != nil {
		return nil, fmt.Errorf("listdbs failure - %v", err)
	}
	return dbs, err
}

// Transact performs the provided Operation's on the database
// RFC 7047 : transact
func (ovs OvsdbClient) Transact(operation ...ovsdb.Operation) ([]ovsdb.OperationResult, error) {
	var reply []ovsdb.OperationResult

	if ok := ovs.Schema.ValidateOperations(operation...); !ok {
		return nil, fmt.Errorf("validation failed for the operation")
	}

	args := ovsdb.NewTransactArgs(ovs.Schema.Name, operation...)
	err := ovs.rpcClient.Call("transact", args, &reply)
	if err != nil {
		return nil, err
	}
	return reply, nil
}

// MonitorAll is a convenience method to monitor every table/column
func (ovs OvsdbClient) MonitorAll(jsonContext interface{}) error {
	requests := make(map[string]ovsdb.MonitorRequest)
	for table, tableSchema := range ovs.Schema.Tables {
		var columns []string
		for column := range tableSchema.Columns {
			columns = append(columns, column)
		}
		requests[table] = ovsdb.MonitorRequest{
			Columns: columns,
			Select:  ovsdb.NewDefaultMonitorSelect(),
		}
	}
	return ovs.Monitor(jsonContext, requests)
}

// MonitorCancel will request cancel a previously issued monitor request
// RFC 7047 : monitor_cancel
func (ovs OvsdbClient) MonitorCancel(jsonContext interface{}) error {
	var reply ovsdb.OperationResult

	args := ovsdb.NewMonitorCancelArgs(jsonContext)

	err := ovs.rpcClient.Call("monitor_cancel", args, &reply)
	if err != nil {
		return err
	}
	if reply.Error != "" {
		return fmt.Errorf("error while executing transaction: %s", reply.Error)
	}
	return nil
}

// Monitor will provide updates for a given table/column
// and populate the cache with them. Subsequent updates will be processed
// by the Update Notifications
// RFC 7047 : monitor
func (ovs OvsdbClient) Monitor(jsonContext interface{}, requests map[string]ovsdb.MonitorRequest) error {
	var reply ovsdb.TableUpdates

	args := ovsdb.NewMonitorArgs(ovs.Schema.Name, jsonContext, requests)
	err := ovs.rpcClient.Call("monitor", args, &reply)
	if err != nil {
		return err
	}
	ovs.Cache.Populate(reply)
	return nil
}

// Echo tests the liveness of the OVSDB connetion
func (ovs *OvsdbClient) Echo() error {
	args := ovsdb.NewEchoArgs()
	var reply []interface{}
	err := ovs.rpcClient.Call("echo", args, &reply)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(args, reply) {
		return fmt.Errorf("incorrect server response: %v, %v", args, reply)
	}
	return nil
}

func (ovs *OvsdbClient) clearConnection() {
	for _, handler := range ovs.handlers {
		if handler != nil {
			handler.Disconnected()
		}
	}
}

func (ovs *OvsdbClient) handleDisconnectNotification() {
	disconnected := ovs.rpcClient.DisconnectNotify()
	<-disconnected
	ovs.clearConnection()
}

// Disconnect will close the OVSDB connection
func (ovs OvsdbClient) Disconnect() {
	close(ovs.stopCh)
	ovs.rpcClient.Close()
}

// Client API interface wrapper functions
// We add this wrapper to allow users to access the API directly on the
// client object

// Ensure client implements API
var _ API = OvsdbClient{}

//Get implements the API interface's Get function
func (ovs OvsdbClient) Get(model model.Model) error {
	return ovs.api.Get(model)
}

//Create implements the API interface's Create function
func (ovs OvsdbClient) Create(models ...model.Model) ([]ovsdb.Operation, error) {
	return ovs.api.Create(models...)
}

//List implements the API interface's List function
func (ovs OvsdbClient) List(result interface{}) error {
	return ovs.api.List(result)
}

//Where implements the API interface's Where function
func (ovs OvsdbClient) Where(m model.Model, conditions ...model.Condition) ConditionalAPI {
	return ovs.api.Where(m, conditions...)
}

//WhereAll implements the API interface's WhereAll function
func (ovs OvsdbClient) WhereAll(m model.Model, conditions ...model.Condition) ConditionalAPI {
	return ovs.api.WhereAll(m, conditions...)
}

//WhereCache implements the API interface's WhereCache function
func (ovs OvsdbClient) WhereCache(predicate interface{}) ConditionalAPI {
	return ovs.api.WhereCache(predicate)
}
