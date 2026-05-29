package postgres

import (
	"strings"

	"github.com/elgnas/dbviz/internal/model"
)

// mapType converts a column's pg_catalog type info into a canonical type with
// the full original type preserved as a suffix, e.g. "string (character
// varying(255))" or "uuid (uuid)" (§2.4).
//
//   - formatType is format_type(atttypid, atttypmod), e.g. "character
//     varying(255)", "numeric(10,2)", "timestamp without time zone", "integer[]".
//   - typName is pg_type.typname, e.g. "varchar", "int4", "uuid", "_int4".
//   - isEnum is true when pg_type.typtype = 'e'.
func mapType(formatType, typName string, isEnum bool) string {
	return canonicalPGType(formatType, typName, isEnum) + " (" + formatType + ")"
}

func canonicalPGType(formatType, typName string, isEnum bool) string {
	if isEnum {
		return model.TypeEnum
	}
	ft := strings.ToLower(strings.TrimSpace(formatType))
	tn := strings.ToLower(strings.TrimSpace(typName))

	// Arrays: typname carries a leading underscore (e.g. "_int4") and
	// format_type ends in "[]".
	if strings.HasPrefix(tn, "_") || strings.HasSuffix(ft, "[]") {
		return model.TypeArray
	}

	// Strip any type modifier, e.g. "character varying(255)" -> "character varying".
	base := ft
	if i := strings.IndexByte(base, '('); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}

	switch base {
	case "uuid":
		return model.TypeUUID
	case "boolean":
		return model.TypeBoolean
	case "smallint", "integer":
		return model.TypeInteger
	case "bigint":
		return model.TypeBigint
	case "numeric", "money":
		return model.TypeDecimal
	case "real", "double precision":
		return model.TypeFloat
	case "character varying", "character", "name":
		return model.TypeString
	case "text", "citext":
		return model.TypeText
	case "timestamp without time zone", "timestamp with time zone":
		return model.TypeTimestamp
	case "date":
		return model.TypeDate
	case "time without time zone", "time with time zone":
		return model.TypeString
	case "json", "jsonb":
		return model.TypeJSON
	case "bytea":
		return model.TypeBinary
	case "point", "line", "polygon", "geometry", "geography":
		return model.TypeGeometry
	}

	// Fall back to typname for cases the format string is unusual.
	switch tn {
	case "uuid":
		return model.TypeUUID
	case "bool":
		return model.TypeBoolean
	case "int2", "int4":
		return model.TypeInteger
	case "int8":
		return model.TypeBigint
	case "numeric":
		return model.TypeDecimal
	case "float4", "float8":
		return model.TypeFloat
	case "varchar", "bpchar":
		return model.TypeString
	case "text":
		return model.TypeText
	case "timestamp", "timestamptz":
		return model.TypeTimestamp
	case "date":
		return model.TypeDate
	case "json", "jsonb":
		return model.TypeJSON
	}

	return model.TypeUnknown
}
