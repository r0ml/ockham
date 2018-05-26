package main

import (
	"database/sql"
	"github.com/aws/aws-sdk-go/aws"

	xlambda "github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/aws/session"
	"fmt"
	"encoding/json"
	"os"
)

// select tablename::varchar as table from pg_tables where schemaname = $1

func getArray(format string, db *sql.DB, cmd string, mm []interface{}) (interface{}, error) {
	rows, err := db.Query(cmd, mm... )

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]interface{}, 0)

	cs, err := rows.Columns()
	if err != nil {
		return "failed to get columns", err
	}

	if format == "array" {
		entrya := make([] interface{}, len(cs))
		for i, col := range cs {
			entrya[i] = col
		}
		res = append(res, entrya)
	}

	values := make([]interface{}, len(cs) )
	valuePtrs := make([]interface{}, len(cs) )

	for rows.Next() {
		for i := 0; i<len(cs); i++ {
			valuePtrs[i] = &values[i]
		}

		err := rows.Scan(valuePtrs...)
		if err != nil {
			return nil, err
		}

		switch format {
		case "array":
			entrya := make([]interface{}, len(cs))
			for i := range cs {
				entrya[i] = values[i]
			}
			res = append(res, entrya)
		default:
			// this creates an entry which is a map of colname:value
			entry := make(map[string]interface{})
			for i, col := range cs {
				// var v interface{}
				val := values[i]
				entry[col] = val
			}

			res = append(res, entry)
		}
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return res, nil
}



func main() {
	remoteFunction := "arn:aws:lambda:us-east-1:844647875270:function:sqlTemplate"

	sql := "select tablename::varchar as table from pg_tables where schemaname = $1"
	sn := os.Args[1]
	args :=  []interface{}{sn}

	// var payload events.APIGatewayProxyRequest
	// payload.Headers["x-conninfo"] = "xx"
	// payload.Headers["x-sqlcmd"] = sql

	aa, ee := json.Marshal(args)
	if ee != nil {
		fmt.Println("marshalling args: %+v", ee)
		return
	}

	payload := map[string]interface{}{"Headers": map[string]interface{}{"x-sqlcmd":sql}, "Body":aa}

	config := aws.Config{ Region: aws.String("us-east-1") }
	sess, err := session.NewSession( &config )
	if err != nil {
		fmt.Println("getting session: %+v", err)
		return
	}
	client := xlambda.New(sess, &config )

	jp, ee := json.Marshal(payload)
	if ee != nil {
		fmt.Println("marshalling payload: %+v", ee)
		return
	}

	z := xlambda.InvokeInput{FunctionName: aws.String(remoteFunction), Payload: jp}
	result, err := client.Invoke(&z)
	if err != nil {
		fmt.Println("error: %+v", err)
	} else {
		fmt.Println("%+v, %+v", *result, string(result.Payload) )
	}
}
