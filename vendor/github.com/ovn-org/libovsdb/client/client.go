package client

import (
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
	"github.com/ovn-org/libovsdb/ovsdb"
)

// OvsdbClient is an OVSDB client
type OvsdbClient struct {
	rpcClient     *rpc2.Client
	Schema        ovsdb.DatabaseSchema
	handlers      []ovsdb.NotificationHandler
	handlersMutex *sync.Mutex
	Cache         *TableCache
	stopCh        chan struct{}
	API           API
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
	defaultTCPAddress  = "127.0.0.1:6640"
	defaultUnixAddress = "/var/run/openvswitch/ovnnb_db.sock"
	SSL                = "ssl"
	TCP                = "tcp"
	UNIX               = "unix"
)

// Connect to ovn, using endpoint in format ovsdb Connection Methods
// If address is empty, use default address for specified protocol
func Connect(endpoints string, database *DBModel, tlsConfig *tls.Config) (*OvsdbClient, error) {
	var c net.Conn
	var err error
	var u *url.URL

	for _, endpoint := range strings.Split(endpoints, ",") {
		if u, err = url.Parse(endpoint); err != nil {
			return nil, err
		}
		// u.Opaque contains the original endPoint with the leading protocol stripped
		// off. For example: endPoint is "tcp:127.0.0.1:6640" and u.Opaque is "127.0.0.1:6640"
		host := u.Opaque
		if len(host) == 0 {
			host = defaultTCPAddress
		}
		switch u.Scheme {
		case UNIX:
			path := u.Path
			if len(path) == 0 {
				path = defaultUnixAddress
			}
			c, err = net.Dial(u.Scheme, path)
		case TCP:
			c, err = net.Dial(u.Scheme, host)
		case SSL:
			c, err = tls.Dial("tcp", host, tlsConfig)
		default:
			err = fmt.Errorf("unknown network protocol %s", u.Scheme)
		}

		if err == nil {
			return newRPC2Client(c, database)
		}
	}

	return nil, fmt.Errorf("failed to connect to endpoints %q: %v", endpoints, err)
}

func newRPC2Client(conn net.Conn, database *DBModel) (*OvsdbClient, error) {
	ovs := newOvsdbClient()
	ovs.rpcClient = rpc2.NewClientWithCodec(jsonrpc.NewJSONCodec(conn))
	ovs.rpcClient.SetBlocking(true)
	ovs.rpcClient.Handle("echo", func(_ *rpc2.Client, args []interface{}, reply *[]interface{}) error {
		return ovs.echo(args, reply)
	})
	ovs.rpcClient.Handle("update", func(_ *rpc2.Client, args []interface{}, _ *[]interface{}) error {
		return ovs.update(args)
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
		if cache, err := newTableCache(schema, database); err == nil {
			ovs.Cache = cache
			ovs.Register(ovs.Cache)
			ovs.API = newAPI(ovs.Cache)
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

// Register registers the supplied NotificationHandler to recieve OVSDB Notifications
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

// Unregister the supplied NotificationHandler to not recieve OVSDB Notifications anymore
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
// Processing "params": [<json-value>, <table-updates>]
func (ovs *OvsdbClient) update(params []interface{}) error {
	if len(params) < 2 {
		return fmt.Errorf("invalid update message")
	}
	// Ignore params[0] as we dont use the <json-value> currently for comparison

	raw, ok := params[1].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid update message")
	}
	var rowUpdates map[string]map[string]ovsdb.RowUpdate

	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &rowUpdates)
	if err != nil {
		return err
	}

	// Update the local DB cache with the tableUpdates
	tableUpdates := getTableUpdatesFromRawUnmarshal(rowUpdates)
	ovs.handlersMutex.Lock()
	defer ovs.handlersMutex.Unlock()
	for _, handler := range ovs.handlers {
		handler.Update(params[0], tableUpdates)
	}

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
			Select: ovsdb.MonitorSelect{
				Initial: true,
				Insert:  true,
				Delete:  true,
				Modify:  true,
			}}
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

	// This totally sucks. Refer to golang JSON issue #6213
	var response map[string]map[string]ovsdb.RowUpdate
	err := ovs.rpcClient.Call("monitor", args, &response)
	reply = getTableUpdatesFromRawUnmarshal(response)
	if err != nil {
		return err
	}
	ovs.Cache.populate(reply)
	return nil
}

func getTableUpdatesFromRawUnmarshal(raw map[string]map[string]ovsdb.RowUpdate) ovsdb.TableUpdates {
	var tableUpdates ovsdb.TableUpdates
	tableUpdates.Updates = make(map[string]ovsdb.TableUpdate)
	for table, update := range raw {
		tableUpdate := ovsdb.TableUpdate{Rows: update}
		tableUpdates.Updates[table] = tableUpdate
	}
	return tableUpdates
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
