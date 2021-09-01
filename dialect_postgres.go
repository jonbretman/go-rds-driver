package rds

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rdsdataservice"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var postgresRegex = regexp.MustCompile("\\$([0-9]+)")

// DialectPostgres is for postgres 10.14 as supported by aurora serverless
type DialectPostgres struct {
}

// MigrateQuery from Postgres to RDS.
func (d *DialectPostgres) MigrateQuery(query string, args []driver.NamedValue) (*rdsdataservice.ExecuteStatementInput, error) {
	// Make sure we're not mixing and matching.
	ordinal := false
	named := false
	for _, arg := range args {
		if arg.Name != "" {
			named = true
		}
		if arg.Ordinal > 0 {
			ordinal = true
		}
		if named && ordinal {
			return nil, ErrNoMixedParams
		}
	}

	// If we're ordinal, convert to named
	if ordinal {
		namedArgs := make([]driver.NamedValue, len(args))
		for i, v := range args {
			namedArgs[i] = driver.NamedValue{
				Name:  strconv.Itoa(v.Ordinal),
				Value: v.Value,
			}
		}
		args = namedArgs

		query = postgresRegex.ReplaceAllStringFunc(query, func(s string) string {
			return strings.Replace(s, "$", ":", 1)
		})

		params, err := ConvertNamedValues(namedArgs)
		return &rdsdataservice.ExecuteStatementInput{
			Parameters: params,
			Sql:        aws.String(query),
		}, err
	}
	params, err := ConvertNamedValues(args)
	return &rdsdataservice.ExecuteStatementInput{
		Parameters: params,
		Sql:        aws.String(query),
	}, err
}

// GetFieldConverter knows how to parse response data.
func (d *DialectPostgres) GetFieldConverter(columnType string) FieldConverter {
	switch strings.ToLower(columnType) {
	case "serial":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			return aws.Int64Value(field.LongValue), nil
		}
	case "bool":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			return aws.BoolValue(field.BooleanValue), nil
		}
	case "bpchar":
		fallthrough
	case "varchar":
		fallthrough
	case "text":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			return aws.StringValue(field.StringValue), nil
		}
	case "int2":
		fallthrough
	case "int4":
		fallthrough
	case "int8":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			return aws.Int64Value(field.LongValue), nil
		}
	case "numeric":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			return strconv.ParseFloat(aws.StringValue(field.StringValue), 64)
		}
	case "float4":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			return aws.Float64Value(field.DoubleValue), nil
		}
	case "date":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			t, err := time.Parse("2006-01-02", aws.StringValue(field.StringValue))
			if err != nil {
				return nil, err
			}
			return t.Format(time.RFC3339), nil
		}
	case "time":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			return aws.StringValue(field.StringValue), nil
		}
	case "timestamp":
		return func(field *rdsdataservice.Field) (interface{}, error) {
			t, err := time.Parse("2006-01-02 15:04:05", aws.StringValue(field.StringValue))
			if err != nil {
				return nil, err
			}
			return t.Format(time.RFC3339), nil
		}
	}
	return func(field *rdsdataservice.Field) (interface{}, error) {
		return nil, fmt.Errorf("unknown type %s, please submit a PR", columnType)
	}
}

// IsIsolationLevelSupported for postgres?
func (d *DialectPostgres) IsIsolationLevelSupported(level driver.IsolationLevel) bool {
	// SupportedIsolationLevels for the dialect
	var SupportedIsolationLevels = map[driver.IsolationLevel]bool{
		driver.IsolationLevel(sql.LevelDefault):         true,
		driver.IsolationLevel(sql.LevelRepeatableRead):  true,
		driver.IsolationLevel(sql.LevelReadCommitted):   true,
		driver.IsolationLevel(sql.LevelReadUncommitted): true,
		driver.IsolationLevel(sql.LevelSerializable):    true,
	}
	_, ok := SupportedIsolationLevels[level]
	return ok
}
