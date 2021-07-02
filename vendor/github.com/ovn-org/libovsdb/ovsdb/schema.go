package ovsdb

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// DatabaseSchema is a database schema according to RFC7047
type DatabaseSchema struct {
	Name    string                 `json:"name"`
	Version string                 `json:"version"`
	Tables  map[string]TableSchema `json:"tables"`
}

// UUIDColumn is a static column that represents the _uuid column, common to all tables
var UUIDColumn = ColumnSchema{
	Type: TypeUUID,
}

// Table returns a TableSchema Schema for a given table and column name
func (schema DatabaseSchema) Table(tableName string) *TableSchema {
	if table, ok := schema.Tables[tableName]; ok {
		return &table
	}
	return nil
}

// Print will print the contents of the DatabaseSchema
func (schema DatabaseSchema) Print(w io.Writer) {
	fmt.Fprintf(w, "%s, (%s)\n", schema.Name, schema.Version)
	for table, tableSchema := range schema.Tables {
		fmt.Fprintf(w, "\t %s", table)
		if len(tableSchema.Indexes) > 0 {
			fmt.Fprintf(w, "(%v)\n", tableSchema.Indexes)
		} else {
			fmt.Fprintf(w, "\n")
		}
		for column, columnSchema := range tableSchema.Columns {
			fmt.Fprintf(w, "\t\t %s => %s\n", column, columnSchema)
		}
	}
}

// ValidateOperations performs basic validation for operations against a DatabaseSchema
func (schema DatabaseSchema) ValidateOperations(operations ...Operation) bool {
	for _, op := range operations {
		table, ok := schema.Tables[op.Table]
		if ok {
			for column := range op.Row {
				if _, ok := table.Columns[column]; !ok {
					if column != "_uuid" && column != "_version" {
						return false
					}
				}
			}
			for _, row := range op.Rows {
				for column := range row {
					if _, ok := table.Columns[column]; !ok {
						if column != "_uuid" && column != "_version" {
							return false
						}
					}
				}
			}
			for _, column := range op.Columns {
				if _, ok := table.Columns[column]; !ok {
					if column != "_uuid" && column != "_version" {
						return false
					}
				}
			}
		} else {
			return false
		}
	}
	return true
}

// TableSchema is a table schema according to RFC7047
type TableSchema struct {
	Columns map[string]*ColumnSchema `json:"columns"`
	Indexes [][]string               `json:"indexes,omitempty"`
}

// Column returns the Column object for a specific column name
func (t TableSchema) Column(columnName string) *ColumnSchema {
	if columnName == "_uuid" {
		return &UUIDColumn
	}
	if column, ok := t.Columns[columnName]; ok {
		return column
	}
	return nil
}

/*RFC7047 defines some atomic-types (e.g: integer, string, etc). However, the Column's type
can also hold other more complex types such as set, enum and map. The way to determine the type
depends on internal, not directly marshallable fields. Therefore, in order to simplify the usage
of this library, we define an ExtendedType that includes all possible column types (including
atomic fields).
*/

//ExtendedType includes atomic types as defined in the RFC plus Enum, Map and Set
type ExtendedType = string

// RefType is used to define the possible RefTypes
type RefType = string

const (
	//Unlimited is used to express unlimited "Max"
	Unlimited int = -1

	//Strong RefType
	Strong RefType = "strong"
	//Weak RefType
	Weak RefType = "weak"

	//ExtendedType associated with Atomic Types

	//TypeInteger is equivalent to 'int'
	TypeInteger ExtendedType = "integer"
	//TypeReal is equivalent to 'float64'
	TypeReal ExtendedType = "real"
	//TypeBoolean is equivalent to 'bool'
	TypeBoolean ExtendedType = "boolean"
	//TypeString is equivalent to 'string'
	TypeString ExtendedType = "string"
	//TypeUUID is equivalent to 'libovsdb.UUID'
	TypeUUID ExtendedType = "uuid"

	//Extended Types used to summarize the interal type of the field.

	//TypeEnum is an enumerator of type defined by Key.Type
	TypeEnum ExtendedType = "enum"
	//TypeMap is a map whose type depend on Key.Type and Value.Type
	TypeMap ExtendedType = "map"
	//TypeSet is a set whose type depend on Key.Type
	TypeSet ExtendedType = "set"
)

// ColumnSchema is a column schema according to RFC7047
type ColumnSchema struct {
	// According to RFC7047, "type" field can be, either an <atomic-type>
	// Or a ColumnTypeObject defined below. To try to simplify the usage, the
	// json message will be parsed manually and Type will indicate the "extended"
	// type. Depending on its value, more information may be available in TypeObj.
	// E.g: If Type == TypeEnum, TypeObj.Key.Enum contains the possible values
	Type      ExtendedType
	TypeObj   *ColumnType
	Ephemeral bool
	Mutable   bool
}

// ColumnType is a type object as per RFC7047
type ColumnType struct {
	Key   *BaseType
	Value *BaseType
	Min   int
	// Unlimited is expressed by the const value Unlimited (-1)
	Max int
}

// BaseType is a base-type structure as per RFC7047
type BaseType struct {
	Type string `json:"type"`
	// Enum will be parsed manually and set to a slice
	// of possible values. They must be type-asserted to the
	// corret type depending on the Type field
	Enum       []interface{} `json:"_"`
	MinReal    float64       `json:"minReal,omitempty"`
	MaxReal    float64       `json:"maxReal,omitempty"`
	MinInteger int           `json:"minInteger,omitempty"`
	MaxInteger int           `json:"maxInteger,omitempty"`
	MinLength  int           `json:"minLength,omitempty"`
	MaxLength  int           `json:"maxLength,omitempty"`
	RefTable   string        `json:"refTable,omitempty"`
	RefType    RefType       `json:"refType,omitempty"`
}

// String returns a string representation of the (native) column type
func (column *ColumnSchema) String() string {
	var flags []string
	var flagStr string
	var typeStr string
	if column.Ephemeral {
		flags = append(flags, "E")
	}
	if column.Mutable {
		flags = append(flags, "M")
	}
	if len(flags) > 0 {
		flagStr = fmt.Sprintf("[%s]", strings.Join(flags, ","))
	}

	switch column.Type {
	case TypeInteger, TypeReal, TypeBoolean, TypeString:
		typeStr = string(column.Type)
	case TypeUUID:
		if column.TypeObj != nil && column.TypeObj.Key != nil {
			typeStr = fmt.Sprintf("uuid [%s (%s)]", column.TypeObj.Key.RefTable, column.TypeObj.Key.RefType)
		} else {
			typeStr = "uuid"
		}

	case TypeEnum:
		typeStr = fmt.Sprintf("enum (type: %s): %v", column.TypeObj.Key.Type, column.TypeObj.Key.Enum)
	case TypeMap:
		typeStr = fmt.Sprintf("[%s]%s", column.TypeObj.Key.Type, column.TypeObj.Value.Type)
	case TypeSet:
		var keyStr string
		if column.TypeObj.Key.Type == TypeUUID {
			keyStr = fmt.Sprintf(" [%s (%s)]", column.TypeObj.Key.RefTable, column.TypeObj.Key.RefType)
		} else {
			keyStr = string(column.TypeObj.Key.Type)
		}
		typeStr = fmt.Sprintf("[]%s (min: %d, max: %d)", keyStr, column.TypeObj.Min, column.TypeObj.Max)
	default:
		panic(fmt.Sprintf("Unsupported type %s", column.Type))
	}

	return strings.Join([]string{typeStr, flagStr}, " ")
}

// UnmarshalJSON unmarshalls a json-formatted column
func (column *ColumnSchema) UnmarshalJSON(data []byte) error {
	// ColumnJSON represents the known json values for a Column
	type ColumnJSON struct {
		TypeRawMsg json.RawMessage `json:"type"`
		Ephemeral  bool            `json:"ephemeral,omitempty"`
		Mutable    bool            `json:"mutable,omitempty"`
	}
	colJSON := ColumnJSON{
		Mutable: true,
	}

	// Unmarshall known keys
	if err := json.Unmarshal(data, &colJSON); err != nil {
		return fmt.Errorf("cannot parse column object %s", err)
	}

	column.Ephemeral = colJSON.Ephemeral
	column.Mutable = colJSON.Mutable

	// 'type' can be a string or an object, let's figure it out
	var typeString string
	if err := json.Unmarshal(colJSON.TypeRawMsg, &typeString); err == nil {
		if !isAtomicType(typeString) {
			return fmt.Errorf("schema contains unknown atomic type %s", typeString)
		}
		// This was an easy one. Use the string as our 'extended' type
		column.Type = typeString
		return nil
	}

	// 'type' can be an object defined as:
	// "key": <base-type>                 required
	// "value": <base-type>               optional
	// "min": <integer>                   optional (default: 1)
	// "max": <integer> or "unlimited"    optional (default: 1)
	column.TypeObj = &ColumnType{
		Key:   &BaseType{},
		Value: nil,
		Max:   1,
		Min:   1,
	}

	// ColumnTypeJSON is used to dynamically decode the ColumnType
	type ColumnTypeJSON struct {
		KeyRawMsg   *json.RawMessage `json:"key,omitempty"`
		ValueRawMsg *json.RawMessage `json:"value,omitempty"`
		Min         int              `json:"min,omitempty"`
		MaxRawMsg   *json.RawMessage `json:"max,omitempty"`
	}
	colTypeJSON := ColumnTypeJSON{
		Min: 1,
	}

	if err := json.Unmarshal(colJSON.TypeRawMsg, &colTypeJSON); err != nil {
		return fmt.Errorf("cannot parse type object: %s", err)
	}

	// Now we have to unmarshall some fields manually because they can store
	// values of different types. Also, in order to really know what native
	// type can store a value of this column, the RFC defines some logic based
	// on the values of 'type'. So, in addition to manually unmarshalling, let's
	// figure out what is the real native type and store it in column.Type for
	// ease of use.

	// 'max' can be an integer or the string "unlimmited". To simplify, use -1
	// as unlimited
	if colTypeJSON.MaxRawMsg != nil {
		var maxString string
		if err := json.Unmarshal(*colTypeJSON.MaxRawMsg, &maxString); err == nil {
			if maxString == "unlimited" {
				column.TypeObj.Max = Unlimited
			} else {
				return fmt.Errorf("unknown max value %s", maxString)
			}
		} else if err := json.Unmarshal(*colTypeJSON.MaxRawMsg, &column.TypeObj.Max); err != nil {
			return fmt.Errorf("cannot parse max field: %s", err)
		}
	}
	column.TypeObj.Min = colTypeJSON.Min

	// 'key' and 'value' can, themselves, be a string or a BaseType.
	// key='<atomic_type>' is equivalent to 'key': {'type': '<atomic_type>'}
	// To simplify things a bit, we'll translate the former to the latter

	if err := json.Unmarshal(*colTypeJSON.KeyRawMsg, &column.TypeObj.Key.Type); err != nil {
		if err := json.Unmarshal(*colTypeJSON.KeyRawMsg, column.TypeObj.Key); err != nil {
			return fmt.Errorf("cannot parse key object: %s", err)
		}
		if err := column.TypeObj.Key.parseEnum(*colTypeJSON.KeyRawMsg); err != nil {
			return err
		}
	}

	if !isAtomicType(column.TypeObj.Key.Type) {
		return fmt.Errorf("schema contains unknown atomic type %s", column.TypeObj.Key.Type)
	}

	// 'value' is optional. If it exists, we know the real native type is a map
	if colTypeJSON.ValueRawMsg != nil {
		column.TypeObj.Value = &BaseType{}
		if err := json.Unmarshal(*colTypeJSON.ValueRawMsg, &column.TypeObj.Value.Type); err != nil {
			if err := json.Unmarshal(*colTypeJSON.ValueRawMsg, &column.TypeObj.Value); err != nil {
				return fmt.Errorf("cannot parse value object: %s", err)
			}
			if err := column.TypeObj.Value.parseEnum(*colTypeJSON.ValueRawMsg); err != nil {
				return err
			}
		}
		if !isAtomicType(column.TypeObj.Value.Type) {
			return fmt.Errorf("schema contains unknown atomic type %s", column.TypeObj.Key.Type)
		}
	}

	// Technially, we have finished unmarshalling. But let's finish infering the native
	if column.TypeObj.Value != nil {
		column.Type = TypeMap
	} else if column.TypeObj.Min != 1 || column.TypeObj.Max != 1 {
		column.Type = TypeSet
	} else if len(column.TypeObj.Key.Enum) > 0 {
		column.Type = TypeEnum
	} else {
		column.Type = column.TypeObj.Key.Type
	}
	return nil
}

// parseEnum decodes the enum field and populates the BaseType.Enum field
func (bt *BaseType) parseEnum(rawData json.RawMessage) error {
	// EnumJSON is used to dynamically decode the Enum values
	type EnumJSON struct {
		Enum interface{} `json:"enum,omitempty"`
	}
	var enumJSON EnumJSON

	if err := json.Unmarshal(rawData, &enumJSON); err != nil {
		return fmt.Errorf("cannot parse enum object: %s (%s)", string(rawData), err)
	}
	// enum is optional
	if enumJSON.Enum == nil {
		return nil
	}

	// 'enum' is a list or a single element representing a list of exactly one element
	switch enumJSON.Enum.(type) {
	case []interface{}:
		// it's an OvsSet
		oSet := enumJSON.Enum.([]interface{})
		innerSet := oSet[1].([]interface{})
		bt.Enum = make([]interface{}, len(innerSet))
		for k, val := range innerSet {
			bt.Enum[k] = val
		}
	default:
		bt.Enum = []interface{}{enumJSON.Enum}
	}
	return nil
}

func isAtomicType(atype string) bool {
	switch atype {
	case TypeInteger, TypeReal, TypeBoolean, TypeString, TypeUUID:
		return true
	default:
		return false
	}
}
