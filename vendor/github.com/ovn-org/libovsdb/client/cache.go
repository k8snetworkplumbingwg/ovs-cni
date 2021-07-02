package client

import (
	"fmt"
	"reflect"
	"sync"

	"log"

	"github.com/ovn-org/libovsdb/ovsdb"
)

const (
	updateEvent = "update"
	addEvent    = "add"
	deleteEvent = "delete"
	bufferSize  = 65536
)

// RowCache is a collections of Models hashed by UUID
type RowCache struct {
	cache map[string]Model
	mutex sync.RWMutex
}

// Row returns one model from the cache by UUID
func (r *RowCache) Row(uuid string) Model {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if row, ok := r.cache[uuid]; ok {
		return row.(Model)
	}
	return nil
}

// Rows returns a list of row UUIDs as strings
func (r *RowCache) Rows() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	var result []string
	for k := range r.cache {
		result = append(result, k)
	}
	return result
}

// Len returns the length of the cache
func (r *RowCache) Len() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return len(r.cache)
}

func newRowCache() *RowCache {
	return &RowCache{
		cache: make(map[string]Model),
		mutex: sync.RWMutex{},
	}
}

// EventHandler can handle events when the contents of the cache changes
type EventHandler interface {
	OnAdd(table string, model Model)
	OnUpdate(table string, old Model, new Model)
	OnDelete(table string, model Model)
}

// EventHandlerFuncs is a wrapper for the EventHandler interface
// It allows a caller to only implement the functions they need
type EventHandlerFuncs struct {
	AddFunc    func(table string, model Model)
	UpdateFunc func(table string, old Model, new Model)
	DeleteFunc func(table string, model Model)
}

// OnAdd calls AddFunc if it is not nil
func (e *EventHandlerFuncs) OnAdd(table string, model Model) {
	if e.AddFunc != nil {
		e.AddFunc(table, model)
	}
}

// OnUpdate calls UpdateFunc if it is not nil
func (e *EventHandlerFuncs) OnUpdate(table string, old, new Model) {
	if e.UpdateFunc != nil {
		e.UpdateFunc(table, old, new)
	}
}

// OnDelete calls DeleteFunc if it is not nil
func (e *EventHandlerFuncs) OnDelete(table string, row Model) {
	if e.DeleteFunc != nil {
		e.DeleteFunc(table, row)
	}
}

// TableCache contains a collection of RowCaches, hashed by name,
// and an array of EventHandlers that respond to cache updates
type TableCache struct {
	cache          map[string]*RowCache
	cacheMutex     sync.RWMutex
	eventProcessor *eventProcessor
	orm            *orm
	dbModel        *DBModel
}

func newTableCache(schema *ovsdb.DatabaseSchema, dbModel *DBModel) (*TableCache, error) {
	if schema == nil || dbModel == nil {
		return nil, fmt.Errorf("tablecache without databasemodel cannot be populated")
	}
	eventProcessor := newEventProcessor(bufferSize)
	return &TableCache{
		cache:          make(map[string]*RowCache),
		eventProcessor: eventProcessor,
		orm:            newORM(schema),
		dbModel:        dbModel,
	}, nil
}

// Table returns the a Table from the cache with a given name
func (t *TableCache) Table(name string) *RowCache {
	t.cacheMutex.RLock()
	defer t.cacheMutex.RUnlock()
	if table, ok := t.cache[name]; ok {
		return table
	}
	return nil
}

// Tables returns a list of table names that are in the cache
func (t *TableCache) Tables() []string {
	t.cacheMutex.RLock()
	defer t.cacheMutex.RUnlock()
	var result []string
	for k := range t.cache {
		result = append(result, k)
	}
	return result
}

// Update implements the update method of the NotificationHandler interface
// this populates the cache with new updates
func (t *TableCache) Update(context interface{}, tableUpdates ovsdb.TableUpdates) {
	if len(tableUpdates.Updates) == 0 {
		return
	}
	t.populate(tableUpdates)
}

// Locked implements the locked method of the NotificationHandler interface
func (t *TableCache) Locked([]interface{}) {
}

// Stolen implements the stolen method of the NotificationHandler interface
func (t *TableCache) Stolen([]interface{}) {
}

// Echo implements the echo method of the NotificationHandler interface
func (t *TableCache) Echo([]interface{}) {
}

// Disconnected implements the disconnected method of the NotificationHandler interface
func (t *TableCache) Disconnected() {
}

// populate adds data to the cache and places an event on the channel
func (t *TableCache) populate(tableUpdates ovsdb.TableUpdates) {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()
	for table := range t.dbModel.Types() {
		updates, ok := tableUpdates.Updates[table]
		if !ok {
			continue
		}
		var tCache *RowCache
		if tCache, ok = t.cache[table]; !ok {
			t.cache[table] = newRowCache()
			tCache = t.cache[table]
		}
		tCache.mutex.Lock()
		for uuid, row := range updates.Rows {
			if !reflect.DeepEqual(row.New, ovsdb.Row{}) {
				newModel, err := t.createModel(table, &row.New, uuid)
				if err != nil {
					panic(err)
				}
				if existing, ok := tCache.cache[uuid]; ok {
					if !reflect.DeepEqual(newModel, existing) {
						tCache.cache[uuid] = newModel
						oldModel, err := t.createModel(table, &row.Old, uuid)
						if err != nil {
							panic(err)
						}
						t.eventProcessor.AddEvent(updateEvent, table, oldModel, newModel)
					}
					// no diff
					continue
				}
				tCache.cache[uuid] = newModel
				t.eventProcessor.AddEvent(addEvent, table, nil, newModel)
				continue
			} else {
				oldModel, err := t.createModel(table, &row.Old, uuid)
				if err != nil {
					panic(err)
				}
				// delete from cache
				delete(tCache.cache, uuid)
				t.eventProcessor.AddEvent(deleteEvent, table, oldModel, nil)
				continue
			}
		}
		tCache.mutex.Unlock()
	}
}

// AddEventHandler registers the supplied EventHandler to recieve cache events
func (t *TableCache) AddEventHandler(handler EventHandler) {
	t.eventProcessor.AddEventHandler(handler)
}

// Run starts the event processing loop. It blocks until the channel is closed.
func (t *TableCache) Run(stopCh <-chan struct{}) {
	t.eventProcessor.Run(stopCh)
}

// event encapsualtes a cache event
type event struct {
	eventType string
	table     string
	old       Model
	new       Model
}

// eventProcessor handles the queueing and processing of cache events
type eventProcessor struct {
	events chan event
	// handlersMutex locks the handlers array when we add a handler or dispatch events
	// we don't need a RWMutex in this case as we only have one thread reading and the write
	// volume is very low (i.e only when AddEventHandler is called)
	handlersMutex sync.Mutex
	handlers      []EventHandler
}

func newEventProcessor(capacity int) *eventProcessor {
	return &eventProcessor{
		events:   make(chan event, capacity),
		handlers: []EventHandler{},
	}
}

// AddEventHandler registers the supplied EventHandler with the eventProcessor
// EventHandlers MUST process events quickly, for example, pushing them to a queue
// to be processed by the client. Long Running handler functions adversely affect
// other handlers and MAY cause loss of data if the channel buffer is full
func (e *eventProcessor) AddEventHandler(handler EventHandler) {
	e.handlersMutex.Lock()
	defer e.handlersMutex.Unlock()
	e.handlers = append(e.handlers, handler)
}

// AddEvent writes an event to the channel
func (e *eventProcessor) AddEvent(eventType string, table string, old Model, new Model) {
	// We don't need to check for error here since there
	// is only a single writer. RPC is run in blocking mode
	event := event{
		eventType: eventType,
		table:     table,
		old:       old,
		new:       new,
	}
	select {
	case e.events <- event:
		// noop
		return
	default:
		log.Print("dropping event because event buffer is full")
	}
}

// Run runs the eventProcessor loop.
// It will block until the stopCh has been closed
// Otherwise it will wait for events to arrive on the event channel
// Once recieved, it will dispatch the event to each registered handler
func (e *eventProcessor) Run(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			return
		case event := <-e.events:
			e.handlersMutex.Lock()
			for _, handler := range e.handlers {
				switch event.eventType {
				case addEvent:
					handler.OnAdd(event.table, event.new)
				case updateEvent:
					handler.OnUpdate(event.table, event.old, event.new)
				case deleteEvent:
					handler.OnDelete(event.table, event.old)
				}
			}
			e.handlersMutex.Unlock()
		}
	}
}

// createModel creates a new Model instance based on the Row information
func (t *TableCache) createModel(tableName string, row *ovsdb.Row, uuid string) (Model, error) {
	table := t.orm.schema.Table(tableName)
	if table == nil {
		return nil, fmt.Errorf("table %s not found", tableName)
	}
	model, err := t.dbModel.newModel(tableName)
	if err != nil {
		return nil, err
	}

	err = t.orm.getRowData(tableName, row, model)
	if err != nil {
		return nil, err
	}

	if uuid != "" {
		ormInfo, err := newORMInfo(table, model)
		if err != nil {
			return nil, err
		}
		if err := ormInfo.setField("_uuid", uuid); err != nil {
			return nil, err
		}
	}

	return model, nil
}
