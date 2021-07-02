package client

import (
	"fmt"
	"reflect"

	"github.com/ovn-org/libovsdb/ovsdb"
)

// ormInfo is a struct that handles ORM information of an object
// The object must have exported tagged fields with the 'ovs'
type ormInfo struct {
	// FieldName indexed by column
	fields map[string]string
	obj    interface{}
	table  *ovsdb.TableSchema
}

// FieldByColumn returns the field value that corresponds to a column
func (oi *ormInfo) fieldByColumn(column string) (interface{}, error) {
	fieldName, ok := oi.fields[column]
	if !ok {
		return nil, fmt.Errorf("column %s not found in orm info", column)
	}
	return reflect.ValueOf(oi.obj).Elem().FieldByName(fieldName).Interface(), nil
}

// FieldByColumn returns the field value that corresponds to a column
func (oi *ormInfo) hasColumn(column string) bool {
	_, ok := oi.fields[column]
	return ok
}

// setField sets the field in the column to the specified value
func (oi *ormInfo) setField(column string, value interface{}) error {
	fieldName, ok := oi.fields[column]
	if !ok {
		return fmt.Errorf("column %s not found in orm info", column)
	}
	fieldValue := reflect.ValueOf(oi.obj).Elem().FieldByName(fieldName)

	if !fieldValue.Type().AssignableTo(reflect.TypeOf(value)) {
		return fmt.Errorf("column %s: native value %v (%s) is not assignable to field %s (%s)",
			column, value, reflect.TypeOf(value), fieldName, fieldValue.Type())
	}
	fieldValue.Set(reflect.ValueOf(value))
	return nil
}

// columnByPtr returns the column name that corresponds to the field by the field's pointer
func (oi *ormInfo) columnByPtr(fieldPtr interface{}) (string, error) {
	fieldPtrVal := reflect.ValueOf(fieldPtr)
	if fieldPtrVal.Kind() != reflect.Ptr {
		return "", ovsdb.NewErrWrongType("ColumnByPointer", "pointer to a field in the struct", fieldPtr)
	}
	offset := fieldPtrVal.Pointer() - reflect.ValueOf(oi.obj).Pointer()
	objType := reflect.TypeOf(oi.obj).Elem()
	for i := 0; i < objType.NumField(); i++ {
		if objType.Field(i).Offset == offset {
			column := objType.Field(i).Tag.Get("ovs")
			if _, ok := oi.fields[column]; !ok {
				return "", fmt.Errorf("field does not have orm column information")
			}
			return column, nil
		}
	}
	return "", fmt.Errorf("field pointer does not correspond to orm struct")
}

// getValidORMIndexes inspects the object and returns the a list of indexes (set of columns) for witch
// the object has non-default values
func (oi *ormInfo) getValidORMIndexes() ([][]string, error) {
	var validIndexes [][]string
	var possibleIndexes [][]string

	possibleIndexes = append(possibleIndexes, []string{"_uuid"})
	possibleIndexes = append(possibleIndexes, oi.table.Indexes...)

	// Iterate through indexes and validate them
OUTER:
	for _, idx := range possibleIndexes {
		for _, col := range idx {
			if !oi.hasColumn(col) {
				continue OUTER
			}
			columnSchema := oi.table.Column(col)
			if columnSchema == nil {
				continue OUTER
			}
			field, err := oi.fieldByColumn(col)
			if err != nil {
				return nil, err
			}
			if !reflect.ValueOf(field).IsValid() || ovsdb.IsDefaultValue(columnSchema, field) {
				continue OUTER
			}
		}
		validIndexes = append(validIndexes, idx)
	}
	return validIndexes, nil
}

// newORMInfo creates a ormInfo structure around an object based on a given table schema
func newORMInfo(table *ovsdb.TableSchema, obj interface{}) (*ormInfo, error) {
	objPtrVal := reflect.ValueOf(obj)
	if objPtrVal.Type().Kind() != reflect.Ptr {
		return nil, ovsdb.NewErrWrongType("NewORMInfo", "pointer to a struct", obj)
	}
	objVal := reflect.Indirect(objPtrVal)
	if objVal.Kind() != reflect.Struct {
		return nil, ovsdb.NewErrWrongType("NewORMInfo", "pointer to a struct", obj)
	}
	objType := objVal.Type()

	fields := make(map[string]string, objType.NumField())
	for i := 0; i < objType.NumField(); i++ {
		field := objType.Field(i)
		colName := field.Tag.Get("ovs")
		if colName == "" {
			// Untagged fields are ignored
			continue
		}
		column := table.Column(colName)
		if column == nil {
			return nil, &ErrORM{
				objType:   objType.String(),
				field:     field.Name,
				fieldType: field.Type.String(),
				fieldTag:  colName,
				reason:    "Column does not exist in schema",
			}
		}

		// Perform schema-based type checking
		expType := ovsdb.NativeType(column)
		if expType != field.Type {
			return nil, &ErrORM{
				objType:   objType.String(),
				field:     field.Name,
				fieldType: field.Type.String(),
				fieldTag:  colName,
				reason:    fmt.Sprintf("Wrong type, column expects %s", expType),
			}
		}
		fields[colName] = field.Name
	}

	return &ormInfo{
		fields: fields,
		obj:    obj,
		table:  table,
	}, nil
}
