package client

import (
	"fmt"
	"reflect"

	"github.com/ovn-org/libovsdb/ovsdb"
)

// ORM offers functions to interact with libovsdb through user-provided native structs.
// The way to specify what field of the struct goes
// to what column in the database id through field a field tag.
// The tag used is "ovs" and has the following structure
// 'ovs:"${COLUMN_NAME}"'
//	where COLUMN_NAME is the name of the column and must match the schema
//
//Example:
//  type MyObj struct {
//  	Name string `ovs:"name"`
//  }
type orm struct {
	schema *ovsdb.DatabaseSchema
}

// ErrORM describes an error in an ORM type
type ErrORM struct {
	objType   string
	field     string
	fieldType string
	fieldTag  string
	reason    string
}

func (e *ErrORM) Error() string {
	return fmt.Sprintf("ORM Error. Object type %s contains field %s (%s) ovs tag %s: %s",
		e.objType, e.field, e.fieldType, e.fieldTag, e.reason)
}

// ErrNoTable describes a error in the provided table information
type ErrNoTable struct {
	table string
}

func (e *ErrNoTable) Error() string {
	return fmt.Sprintf("Table not found: %s", e.table)
}

// NewErrNoTable creates a new ErrNoTable
func NewErrNoTable(table string) error {
	return &ErrNoTable{
		table: table,
	}
}

// newORM returns a new ORM
func newORM(schema *ovsdb.DatabaseSchema) *orm {
	return &orm{
		schema: schema,
	}
}

// GetRowData transforms a Row to a struct based on its tags
// The result object must be given as pointer to an object with the right tags
func (o orm) getRowData(tableName string, row *ovsdb.Row, result interface{}) error {
	if row == nil {
		return nil
	}
	return o.getData(tableName, row.Fields, result)
}

// GetData transforms a map[string]interface{} containing OvS types (e.g: a ResultRow
// has this format) to orm struct
// The result object must be given as pointer to an object with the right tags
func (o orm) getData(tableName string, ovsData map[string]interface{}, result interface{}) error {
	table := o.schema.Table(tableName)
	if table == nil {
		return NewErrNoTable(tableName)
	}

	ormInfo, err := newORMInfo(table, result)
	if err != nil {
		return err
	}

	for name, column := range table.Columns {
		if !ormInfo.hasColumn(name) {
			// If provided struct does not have a field to hold this value, skip it
			continue
		}

		ovsElem, ok := ovsData[name]
		if !ok {
			// Ignore missing columns
			continue
		}

		nativeElem, err := ovsdb.OvsToNative(column, ovsElem)
		if err != nil {
			return fmt.Errorf("table %s, column %s: failed to extract native element: %s",
				tableName, name, err.Error())
		}

		if err := ormInfo.setField(name, nativeElem); err != nil {
			return err
		}
	}
	return nil
}

// newRow transforms an orm struct to a map[string] interface{} that can be used as libovsdb.Row
// By default, default or null values are skipped. This behaviour can be modified by specifying
// a list of fields (pointers to fields in the struct) to be added to the row
func (o orm) newRow(tableName string, data interface{}, fields ...interface{}) (map[string]interface{}, error) {
	table := o.schema.Table(tableName)
	if table == nil {
		return nil, NewErrNoTable(tableName)
	}
	ormInfo, err := newORMInfo(table, data)
	if err != nil {
		return nil, err
	}

	ovsRow := make(map[string]interface{}, len(table.Columns))
	for name, column := range table.Columns {
		nativeElem, err := ormInfo.fieldByColumn(name)
		if err != nil {
			// If provided struct does not have a field to hold this value, skip it
			continue
		}

		// add specific fields
		if len(fields) > 0 {
			found := false
			for _, f := range fields {
				col, err := ormInfo.columnByPtr(f)
				if err != nil {
					return nil, err
				}
				if col == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if len(fields) == 0 && ovsdb.IsDefaultValue(column, nativeElem) {
			continue
		}
		ovsElem, err := ovsdb.NativeToOvs(column, nativeElem)
		if err != nil {
			return nil, fmt.Errorf("table %s, column %s: failed to generate ovs element. %s", tableName, name, err.Error())
		}
		ovsRow[name] = ovsElem
	}
	return ovsRow, nil
}

// newCondition returns a list of conditions that match a given object
// A list of valid columns that shall be used as a index can be provided.
// If none are provided, we will try to use object's field that matches the '_uuid' ovs tag
// If it does not exist or is null (""), then we will traverse all of the table indexes and
// use the first index (list of simultaneously unique columnns) for witch the provided ORM
// object has valid data. The order in which they are traversed matches the order defined
// in the schema.
// By `valid data` we mean non-default data.
func (o orm) newCondition(tableName string, data interface{}, fields ...interface{}) ([]interface{}, error) {
	var conditions []interface{}
	var condIndex [][]string

	table := o.schema.Table(tableName)
	if table == nil {
		return nil, NewErrNoTable(tableName)
	}

	ormInfo, err := newORMInfo(table, data)
	if err != nil {
		return nil, err
	}

	// If index is provided, use it. If not, obtain the valid indexes from the ORM info
	if len(fields) > 0 {
		providedIndex := []string{}
		for i := range fields {
			if col, err := ormInfo.columnByPtr(fields[i]); err == nil {
				providedIndex = append(providedIndex, col)
			} else {
				return nil, err
			}
		}
		condIndex = append(condIndex, providedIndex)
	} else {
		var err error
		condIndex, err = ormInfo.getValidORMIndexes()
		if err != nil {
			return nil, err
		}
	}

	if len(condIndex) == 0 {
		return nil, fmt.Errorf("failed to find a valid index")
	}

	// Pick the first valid index
	for _, col := range condIndex[0] {
		field, err := ormInfo.fieldByColumn(col)
		if err != nil {
			return nil, err
		}

		column := table.Column(col)
		if column == nil {
			return nil, fmt.Errorf("column %s not found", col)
		}
		ovsVal, err := ovsdb.NativeToOvs(column, field)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, []interface{}{col, "==", ovsVal})
	}
	return conditions, nil
}

// equalFields compares two ORM objects.
// The indexes to use for comparison are, the _uuid, the table indexes and the columns that correspond
// to the ORM fields pointed to by 'fields'. They must be pointers to fields on the first ORM element (i.e: one)
func (o orm) equalFields(tableName string, one, other interface{}, fields ...interface{}) (bool, error) {
	indexes := []string{}

	table := o.schema.Table(tableName)
	if table == nil {
		return false, NewErrNoTable(tableName)
	}

	info, err := newORMInfo(table, one)
	if err != nil {
		return false, err
	}
	for _, f := range fields {
		col, err := info.columnByPtr(f)
		if err != nil {
			return false, err
		}
		indexes = append(indexes, col)
	}
	return o.equalIndexes(table, one, other, indexes...)
}

// newMutation creates a RFC7047 mutation object based on an ORM object and the mutation fields (in native format)
// It takes care of field validation against the column type
func (o orm) newMutation(tableName string, data interface{}, column string, mutator ovsdb.Mutator, value interface{}) ([]interface{}, error) {
	table := o.schema.Table(tableName)
	if table == nil {
		return nil, NewErrNoTable(tableName)
	}

	ormInfo, err := newORMInfo(table, data)
	if err != nil {
		return nil, err
	}

	// Check the column exists in the object
	if !ormInfo.hasColumn(column) {
		return nil, fmt.Errorf("mutation contains column %s that does not exist in object %v", column, data)
	}
	// Check that the mutation is valid
	columnSchema := table.Column(column)
	if columnSchema == nil {
		return nil, fmt.Errorf("column %s not found", column)
	}
	if err := ovsdb.ValidateMutation(columnSchema, mutator, value); err != nil {
		return nil, err
	}

	var ovsValue interface{}
	if mutator == "delete" && columnSchema.Type == ovsdb.TypeMap {
		// It's OK to cast the value to a list of elemets because validation has passed
		ovsSet, err := ovsdb.NewOvsSet(value)
		if err != nil {
			return nil, err
		}
		ovsValue = ovsSet
	} else {
		ovsValue, err = ovsdb.NativeToOvs(columnSchema, value)
		if err != nil {
			return nil, err
		}
	}

	return []interface{}{column, mutator, ovsValue}, nil
}

// equalIndexes returns whether both models are equal from the DB point of view
// Two objectes are considered equal if any of the following conditions is true
// They have a field tagged with column name '_uuid' and their values match
// For any of the indexes defined in the Table Schema, the values all of its columns are simultaneously equal
// (as per RFC7047)
// The values of all of the optional indexes passed as variadic parameter to this function are equal.
func (o orm) equalIndexes(table *ovsdb.TableSchema, one, other interface{}, indexes ...string) (bool, error) {
	match := false

	oneOrmInfo, err := newORMInfo(table, one)
	if err != nil {
		return false, err
	}
	otherOrmInfo, err := newORMInfo(table, other)
	if err != nil {
		return false, err
	}

	oneIndexes, err := oneOrmInfo.getValidORMIndexes()
	if err != nil {
		return false, err
	}

	otherIndexes, err := otherOrmInfo.getValidORMIndexes()
	if err != nil {
		return false, err
	}

	oneIndexes = append(oneIndexes, indexes)
	otherIndexes = append(otherIndexes, indexes)

	for _, lidx := range oneIndexes {
		for _, ridx := range otherIndexes {
			if reflect.DeepEqual(ridx, lidx) {
				// All columns in an index must be simultaneously equal
				for _, col := range lidx {
					if !oneOrmInfo.hasColumn(col) || !otherOrmInfo.hasColumn(col) {
						break
					}
					lfield, err := oneOrmInfo.fieldByColumn(col)
					if err != nil {
						return false, err
					}
					rfield, err := otherOrmInfo.fieldByColumn(col)
					if err != nil {
						return false, err
					}
					if reflect.DeepEqual(lfield, rfield) {
						match = true
					} else {
						match = false
						break
					}
				}
				if match {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
