package client

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/ovn-org/libovsdb/ovsdb"
)

const (
	opInsert string = "insert"
	opMutate string = "mutate"
	opUpdate string = "insert"
	opDelete string = "delete"
)

// API defines basic operations to interact with the database
type API interface {
	// List populates a slice of Models objects based on their type
	// The function parameter must be a pointer to a slice of Models
	// If the slice is null, the entire cache will be copied into the slice
	// If it has a capacity != 0, only 'capacity' elements will be filled in
	List(result interface{}) error

	// Create a Condition from a Function that is used to filter cached data
	// The function must accept a Model implementation and return a boolean. E.g:
	// ConditionFromFunc(func(l *LogicalSwitch) bool { return l.Enabled })
	ConditionFromFunc(predicate interface{}) Condition

	// Create a Condition from a Model's data. It uses the database indexes
	// to search the most apropriate field to use for matches and conditions
	// Optionally, a list of fields can indicate an alternative index
	ConditionFromModel(Model, ...interface{}) Condition

	// Create a ConditionalAPI from a Condition
	Where(condition Condition) ConditionalAPI

	// Get retrieves a model from the cache
	// The way the object will be fetch depends on the data contained in the
	// provided model and the indexes defined in the associated schema
	// For more complex ways of searching for elements in the cache, the
	// preferred way is Where({condition}).List()
	Get(Model) error

	// Create returns the operation needed to add a model to the Database
	// Only fields with non-default values will be added to the transaction
	// If the field associated with column "_uuid" has some content, it will be
	// treated as named-uuid
	Create(Model) (*ovsdb.Operation, error)
}

// ConditionalAPI is an interface used to perform operations that require / use Conditions
type ConditionalAPI interface {
	// List uses the condition to search on the cache and populates
	// the slice of Models objects based on their type
	List(result interface{}) error

	// Mutate returns the operations needed to perform the mutation specified
	// By the model and the list of Mutation objects
	// Depending on the Condition, it might return one or many operations
	Mutate(Model, []Mutation) ([]ovsdb.Operation, error)

	// Update returns the operations needed to update any number of rows according
	// to the data in the given model.
	// By default, all the non-default values contained in model will be updated.
	// Optional fields can be passed (pointer to fields in the model) to select the
	// the fields to be updated
	Update(Model, ...interface{}) ([]ovsdb.Operation, error)

	// Delete returns the Operations needed to delete the models seleted via the condition
	Delete() ([]ovsdb.Operation, error)
}

// Mutation is a type that represents a OVSDB Mutation
type Mutation struct {
	// Pointer to the field of the model that shall be mutated
	Field interface{}
	// String representing the mutator (as per RFC7047)
	Mutator ovsdb.Mutator
	// Value to use in the mutation
	Value interface{}
}

// InputTypeError is used to report the user provided parameter has the wrong type
type InputTypeError struct {
	inputType reflect.Type
	reason    string
}

func (e *InputTypeError) Error() string {
	return fmt.Sprintf("Wrong parameter type (%s): %s", e.inputType, e.reason)
}

// ConditionError is a wrapper around an error that is used to
// indicate the error occurred during condition creation
type ConditionError struct {
	err string
}

func (c ConditionError) Error() string {
	return fmt.Sprintf("Condition Error: %s", c.err)
}
func (c ConditionError) String() string {
	return c.Error()
}

// ErrNotFound is used to inform the object or table was not found in the cache
var ErrNotFound = errors.New("object not found")

// api struct implements both API and ConditionalAPI
// Where() can be used to create a ConditionalAPI api
type api struct {
	cache *TableCache
	cond  Condition
}

// List populates a slice of Models given as parameter based on the configured Condition
func (a api) List(result interface{}) error {
	resultPtr := reflect.ValueOf(result)
	if resultPtr.Type().Kind() != reflect.Ptr {
		return &InputTypeError{resultPtr.Type(), "Expected pointer to slice of valid Models"}
	}

	resultVal := reflect.Indirect(resultPtr)
	if resultVal.Type().Kind() != reflect.Slice {
		return &InputTypeError{resultPtr.Type(), "Expected pointer to slice of valid Models"}
	}

	table, err := a.getTableFromModel(reflect.New(resultVal.Type().Elem()).Interface())
	if err != nil {
		return err
	}

	if a.cond != nil && a.cond.Table() != table {
		return &InputTypeError{resultPtr.Type(),
			fmt.Sprintf("Table derived from input type (%s) does not match Table from Condition (%s)", table, a.cond.Table())}
	}

	tableCache := a.cache.Table(table)
	if tableCache == nil {
		return ErrNotFound
	}

	// If given a null slice, fill it in the cache table completely, if not, just up to
	// its capability
	if resultVal.IsNil() {
		resultVal.Set(reflect.MakeSlice(resultVal.Type(), 0, tableCache.Len()))
	}
	i := resultVal.Len()

	for _, row := range tableCache.Rows() {
		elem := tableCache.Row(row)
		if i >= resultVal.Cap() {
			break
		}

		if a.cond != nil {
			if matches, err := a.cond.Matches(elem); err != nil {
				return err
			} else if !matches {
				continue
			}
		}

		resultVal.Set(reflect.Append(resultVal, reflect.Indirect(reflect.ValueOf(elem))))
		i++
	}
	return nil
}

// Where returns a conditionalAPI based a Condition
func (a api) Where(condition Condition) ConditionalAPI {
	return newConditionalAPI(a.cache, condition)
}

// ConditionFactory interface implementation
// FromFunc returns a Condition from a function
func (a api) ConditionFromFunc(predicate interface{}) Condition {
	table, err := a.getTableFromFunc(predicate)
	if err != nil {
		return newErrorCondition(err)
	}

	condition, err := newPredicateCond(table, a.cache, predicate)
	if err != nil {
		return newErrorCondition(err)
	}
	return condition
}

// FromModel returns a Condition from a model and a list of fields
func (a api) ConditionFromModel(model Model, fields ...interface{}) Condition {
	tableName, err := a.getTableFromModel(model)
	if tableName == "" {
		return newErrorCondition(err)
	}
	condition, err := newIndexCondition(a.cache.orm, tableName, model, fields...)
	if err != nil {
		return newErrorCondition(err)
	}
	return condition
}

// Get is a generic Get function capable of returning (through a provided pointer)
// a instance of any row in the cache.
// 'result' must be a pointer to an Model that exists in the DBModel
//
// The way the cache is search depends on the fields already populated in 'result'
// Any table index (including _uuid) will be used for comparison
func (a api) Get(model Model) error {
	table, err := a.getTableFromModel(model)
	if err != nil {
		return err
	}

	tableCache := a.cache.Table(table)
	if tableCache == nil {
		return ErrNotFound
	}

	// If model contains _uuid value, we can access it via cache index
	ormInfo, err := newORMInfo(a.cache.orm.schema.Table(table), model)
	if err != nil {
		return err
	}
	if uuid, err := ormInfo.fieldByColumn("_uuid"); err != nil && uuid != nil {
		if found := tableCache.Row(uuid.(string)); found == nil {
			return ErrNotFound
		} else {
			reflect.ValueOf(model).Elem().Set(reflect.Indirect(reflect.ValueOf(found)))
			return nil
		}
	}

	// Look across the entire cache for table index equality
	for _, row := range tableCache.Rows() {
		elem := tableCache.Row(row)
		equal, err := a.cache.orm.equalFields(table, model, elem.(Model))
		if err != nil {
			return err
		}
		if equal {
			reflect.ValueOf(model).Elem().Set(reflect.Indirect(reflect.ValueOf(elem)))
			return nil
		}
	}
	return ErrNotFound
}

// Create is a generic function capable of creating any row in the DB
// A valud Model (pointer to object) must be provided.
func (a api) Create(model Model) (*ovsdb.Operation, error) {
	var namedUUID string
	var err error

	tableName, err := a.getTableFromModel(model)
	if err != nil {
		return nil, err
	}
	table := a.cache.orm.schema.Table(tableName)

	// Read _uuid field, and use it as named-uuid
	info, err := newORMInfo(table, model)
	if err != nil {
		return nil, err
	}
	if uuid, err := info.fieldByColumn("_uuid"); err == nil {
		namedUUID = uuid.(string)
	} else {
		return nil, err
	}

	row, err := a.cache.orm.newRow(tableName, model)
	if err != nil {
		return nil, err
	}

	insertOp := ovsdb.Operation{
		Op:       opInsert,
		Table:    tableName,
		Row:      row,
		UUIDName: namedUUID,
	}
	return &insertOp, nil
}

// Mutate returns the operations needed to transform the one Model into another one
func (a api) Mutate(model Model, mutationObjs []Mutation) ([]ovsdb.Operation, error) {
	var mutations []interface{}
	var operations []ovsdb.Operation

	tableName := a.cache.dbModel.FindTable(reflect.ValueOf(model).Type())
	table := a.cache.orm.schema.Table(tableName)
	if table == nil {
		return nil, fmt.Errorf("schema error: table not found in Database Model for type %s", reflect.TypeOf(model))
	}

	conditions, err := a.cond.Generate()
	if err != nil {
		return nil, err
	}

	info, err := newORMInfo(table, model)
	if err != nil {
		return nil, err
	}

	for _, mobj := range mutationObjs {
		col, err := info.columnByPtr(mobj.Field)
		if err != nil {
			return nil, err
		}

		mutation, err := a.cache.orm.newMutation(tableName, model, col, mobj.Mutator, mobj.Value)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, mutation)
	}

	for _, condition := range conditions {
		operations = append(operations,
			ovsdb.Operation{
				Op:        opMutate,
				Table:     tableName,
				Mutations: mutations,
				Where:     condition,
			})
	}
	return operations, nil
}

// Update is a generic function capable of updating any field in any row in the database
// Additional fields can be passed (variadic opts) to indicate fields to be updated
func (a api) Update(model Model, fields ...interface{}) ([]ovsdb.Operation, error) {
	var operations []ovsdb.Operation
	table, err := a.getTableFromModel(model)
	if err != nil {
		return nil, err
	}

	conditions, err := a.cond.Generate()
	if err != nil {
		return nil, err
	}

	row, err := a.cache.orm.newRow(table, model, fields...)
	if err != nil {
		return nil, err
	}

	for _, condition := range conditions {
		operations = append(operations, ovsdb.Operation{
			Op:    opUpdate,
			Table: table,
			Row:   row,
			Where: condition,
		})
	}
	return operations, nil
}

// Delete returns the Operation needed to delete the selected models from the database
func (a api) Delete() ([]ovsdb.Operation, error) {
	var operations []ovsdb.Operation
	conditions, err := a.cond.Generate()
	if err != nil {
		return nil, err
	}

	for _, condition := range conditions {
		operations = append(operations, ovsdb.Operation{
			Op:    opDelete,
			Table: a.cond.Table(),
			Where: condition,
		})
	}

	return operations, nil
}

// getTableFromModel returns the table name from a Model object after performing
// type verifications on the model
func (a api) getTableFromModel(model interface{}) (string, error) {
	if _, ok := model.(Model); !ok {
		return "", &InputTypeError{reflect.TypeOf(model), "Type does not implement Model interface"}
	}

	table := a.cache.dbModel.FindTable(reflect.TypeOf(model))
	if table == "" {
		return "", &InputTypeError{reflect.TypeOf(model), "Model not found in Database Model"}
	}

	return table, nil
}

// getTableFromModel returns the table name from a the predicate after performing
// type verifications
func (a api) getTableFromFunc(predicate interface{}) (string, error) {
	predType := reflect.TypeOf(predicate)
	if predType == nil || predType.Kind() != reflect.Func {
		return "", &InputTypeError{predType, "Expected function"}
	}
	if predType.NumIn() != 1 || predType.NumOut() != 1 || predType.Out(0).Kind() != reflect.Bool {
		return "", &InputTypeError{predType, "Expected func(Model) bool"}
	}

	modelInterface := reflect.TypeOf((*Model)(nil)).Elem()
	modelType := predType.In(0)
	if !modelType.Implements(modelInterface) {
		return "", &InputTypeError{predType,
			fmt.Sprintf("Type %s does not implement Model interface", modelType.String())}
	}

	table := a.cache.dbModel.FindTable(modelType)
	if table == "" {
		return "", &InputTypeError{predType,
			fmt.Sprintf("Model %s not found in Database Model", modelType.String())}
	}
	return table, nil
}

// newAPI returns a new API to interact with the database
func newAPI(cache *TableCache) API {
	return api{
		cache: cache,
	}
}

// newConditionalAPI returns a new ConditionalAPI to interact with the database
func newConditionalAPI(cache *TableCache, cond Condition) ConditionalAPI {
	return api{
		cache: cache,
		cond:  cond,
	}
}
