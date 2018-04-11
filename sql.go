package main

import (
	"fmt"
	"log"
	"context"
	"encoding/base64"
	"encoding/json"
	"database/sql"

	"os"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	_ "github.com/lib/pq"

	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws"
)


func getList(format string, db *sql.DB, cmd string, mm []interface{}) (interface{}, error) {
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

// This will decrypt the passed in string.  Clients of this lambda need to know how to encrypt connection strings
// If the connection strings are stored as environment or stage variables, the expectation is that they are
// encrypted as well.
func kmsAuth(secr string) string {
	config := aws.Config{}
	sess, _ := session.NewSession( &config )
	svc := kms.New( sess, &config)
	b, _ := base64.StdEncoding.DecodeString(secr)
	k := kms.DecryptInput{CiphertextBlob: b}
	t, h := svc.Decrypt(&k)
	if h != nil {
		log.Println(h)
		return "" // this will certainly cause a problem with trying to connect
	}
	return string(t.Plaintext)
}


/* There are at least two cases:
	1) I'm actually hooked up to an API Gateway.  In this case, I get the connection info from a stage variable
       and the sql command from an environment variable

	2) I'm being called via a proxy from an API Gateway.  In this case, I get the connection info from an HTTP header,
       and the sql command likewise

    In order to avoid leakage, if the stage variables are not set for connection info, then I'll look at the
    HTTP headers.  Otherwise, I will not look at them?   Ditto, if the SQL environment variable is set, I'll look it,
    else I'll look at the HTTP header
 */

func handleSQLRequest(_ context.Context, payload events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	conninfo := payload.StageVariables["CONNINFO"]
	if conninfo == "" {
		conninfo = payload.Headers["x-conninfo"]
	}

	sqlcmd := os.Getenv("SQLCMD")
	if sqlcmd == "" {
		sqlcmd = payload.Headers["x-sqlcmd"]
	}

	db := conns[conninfo]
	if db == nil {
		dci := kmsAuth(conninfo)
		dbx, err := sql.Open("postgres", dci)
		if err != nil {
			return errorResponse("failed to connect", err)
		}
		conns[conninfo]=dbx
		db = dbx
	}

	var args = make([]interface{}, 0)

	if len(payload.Body) > 0 {
		err := json.Unmarshal([]byte(payload.Body), &args)

		// if the unmarshal fails -- ignore it
		// if the unmarshal succeeds -- if it is an array, use it
		// if the unmarshal succeeds -- if it is a dictionary, extract x1, x2, x3 ... as an array

		// if there are pathParameters, extract x1, x2, x3 ... as an array
		// if there are queryParameters, extract x1, x2, x3 ... as an array

		// precedence is:  pathParameters are overridden by query pararameters which are overridden by post body

		if err != nil {
			// return errorResponse("unmarshalling body", err)
			args = make([]interface{}, 0)
		}
	}

	qp := payload.QueryStringParameters
	pp := payload.PathParameters

	for i := 1; i<10 ; i++ {
		j := fmt.Sprintf("x%d", i)
		if val, ok := qp[j]; ok {
			for k := len(args); k < i; k++ { args = append(args,nil) }
			args[i-1]=val
		}
		if val, ok := pp[j]; ok {
			for k := len(args); k < i; k++ { args = append(args,nil) }
			args[i-1]=val
		}
	}

	fmtx, ok := qp["format"]
	if !ok { fmtx = "map" }

	res, err := getList(fmtx, db, sqlcmd , args )
	if err != nil {
		return errorResponse("getting data", err)
	}

	resx, err := json.MarshalIndent(res,""," ")
	if err != nil {
		return errorResponse("jsoning data", err)
	}

	return events.APIGatewayProxyResponse{
		Body:       string(resx),
		Headers:    map[string]string{},
		StatusCode: http.StatusOK,
	}, nil
}

func errorResponse(s string, err error) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse {
		Body: fmt.Sprintf("%s: %s", s, err),
		StatusCode: http.StatusBadRequest,
	}, nil
}

// I keep a map of encrypted connection strings to open connections here
var conns map[string]*sql.DB

func main() {
	conns = make(map[string]*sql.DB)
	lambda.Start(handleSQLRequest)
}
