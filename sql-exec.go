package main

import (
	"fmt"
	"context"
	"encoding/json"

	"os"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	// _ "github.com/lib/pq"
	// "database/sql"

	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws"
	"log"

	"github.com/jackc/pgx"
)


func getResult(format string, conn *pgx.Conn, cmd string, mm []interface{}) (int64, error) {
	ct, err := conn.Exec(cmd, mm...)
	if err != nil {
		return -1, err
	}
	return ct.RowsAffected(), nil
}

// This will decrypt the passed in string.  Clients of this lambda need to know how to encrypt connection strings
// If the connection strings are stored as environment or stage variables, the expectation is that they are
// encrypted as well.
func secret(secr string) string {
	config := aws.Config{}
	sess, _ := session.NewSession( &config )

	/*
	svc := kms.New( sess, &config)
	b, _ := base64.StdEncoding.DecodeString(secr)
	k := kms.DecryptInput{CiphertextBlob: b}
	t, h := svc.Decrypt(&k)
	if h != nil {
		log.Println(h)
		return "" // this will certainly cause a problem with trying to connect
	}
	*/

	svc := secretsmanager.New(sess, &config)
	k := secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secr),
	}
	a, t := svc.GetSecretValue(&k)
	if t != nil {
		log.Println("getting secret value: ", secr, t)
		return ""
	}
	b := *a.SecretString
	log.Println("secret: ", secr, b)
	return b
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

func handleSQLCmd(_ context.Context, payload events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// fmt.Printf( "payload: %+v\n", payload)

	conninfo := payload.StageVariables["CONNINFO"]
	if conninfo == "" {
		conninfo = payload.Headers["x-conninfo"]
	}

	sqlcmd := os.Getenv("SQLCMD")
	if sqlcmd == "" {
		sqlcmd = payload.Headers["x-sqlcmd"]
	}


	v := []string {"database", "host", "user", "port", "passwd" }
	r := []string{}

	for _, vv := range v {
		r = append(r, secret(conninfo+"/"+vv))
	}

	dci := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", r[2], r[4], r[1], r[3], r[0])

	// now we set up for pgx
	var notices []string
	conf, err := pgx.ParseURI(dci)
	if (err != nil ) {
		return errorRsp("failed to parse URI", err)
	}
	conf.LogLevel = pgx.LogLevelInfo
	conf.OnNotice = func(c *pgx.Conn, n *pgx.Notice) {
		notices = append(notices, n.Severity+": "+n.Message)
	}
	conn, err := pgx.Connect(conf)
	if (err != nil ) {
		return errorRsp("failed to connect to db", err)
	}

	defer conn.Close()

	if conn == nil {
		return errorRsp("failed to get connection" , nil)
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
			// return errorRsp("unmarshalling body", err)
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

	res, err := getResult(fmtx, conn, sqlcmd , args )
	if err != nil {
		return errorRsp("getting data", err)
	}

	resx, err := json.MarshalIndent( map[string]interface{}{"rows": res, "notices":notices} ,""," ")
	if err != nil {
		return errorRsp("jsoning data", err)
	}

	return events.APIGatewayProxyResponse{
		Body:       string(resx),
		Headers:    map[string]string{},
		StatusCode: http.StatusOK,
	}, nil
}

func errorRsp(s string, err error) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse {
		Body: fmt.Sprintf("%s: %s", s, err),
		StatusCode: http.StatusBadRequest,
	}, nil
}

// I keep a map of encrypted connection strings to open connections here
// var xconns map[string]*pgx.Conn
// for local testing, setenv LOCALTEST to the local postgresql connection URI
func main() {
	lt := os.Getenv("LOCALTEST")
	if lt != "" {
		var notices []string
		conf, err := pgx.ParseURI(lt)
		if (err != nil ) {
			println("failed to parse URI", err)
			return
		}
		conf.LogLevel = pgx.LogLevelInfo
		conf.OnNotice = func(c *pgx.Conn, n *pgx.Notice) {
			notices = append(notices, n.Severity+": "+n.Message)
		}
		conn, err := pgx.Connect(conf)
        if (err != nil ) {
			println("failed to connect to db", err)
			return
		}

		jj := make([]interface{},0)
		for _, k := range os.Args[2:] {
			jj = append(jj, k)
		}
		ct, err := conn.Exec(os.Args[1], jj...)
		if err != nil {
			fmt.Println("executing db command: %+v", err)
			return
		}

		fmt.Printf("rows affected: %d\n", ct.RowsAffected() )
		for i,v := range notices {
			fmt.Printf("%d %v\n", i,  v)
		}
		return
	}

	// xconns = make(map[string]*pgx.Conn)
	lambda.Start(handleSQLCmd)
}
