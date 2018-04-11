package main

import (
	"fmt"
	"context"
	"os"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	xlambda "github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/aws/session"
	"sort"
	"log"
)

/* This could deploy either:
    a) A "proxy" lambda which calls the SQL lambda or
	b) A SQL lambda on the VPN
 */

func handleReflectRequest(_ context.Context, r events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	restApiId := r.RequestContext.APIID
	account := r.RequestContext.AccountID
	stage := r.RequestContext.Stage

	vs, e := url.ParseQuery(string( r.Body))

	if e != nil {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("%v", e),
			StatusCode: http.StatusBadGateway,
		}, nil
	}

	cmd := vs.Get("command")
	gatewayPath := vs.Get("path")

	g, e := doReflect(gatewayPath, cmd, restApiId, account, stage)
	return g,nil
}

func sErr(m string, err error) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		Body:       fmt.Sprintf("%s: %s", m, err),
		Headers: map[string]string{},
		StatusCode: http.StatusBadRequest,
	}, err
}

type Reflection struct {
	Path string
	Name string
	SQL string
}

type Nodes []Reflection;

func (a Nodes) Len() int           { return len(a) }
func (a Nodes) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Nodes) Less(i, j int) bool { return a[i].Path < a[j].Path }


func doReflect(gatewayPath string, cmd string,
	restApiID string, account string, stage string) (events.APIGatewayProxyResponse, error) {

	config := aws.Config{}

	sess, err := session.NewSession( &config )

	if err != nil {
		return sErr("getting session", err)
	}

	client := xlambda.New(sess, &config)
	aclient := apigateway.New(sess, &config )

	// log.Printf("invoke input: %s\n\n%+v\n\n", remoteFunction, string(payload) )
	// ss := strings.Split(gatewayPath, "/")
	var rr []*apigateway.Resource
	var pos *string

	for {

		gri := apigateway.GetResourcesInput{
			RestApiId: aws.String(restApiID),
			Limit:     aws.Int64(25),
			Position:  pos,
		}

		rrx, e := aclient.GetResources(&gri)
		if e != nil {
			return sErr("couldn't get resources", e)
		}
		rr = append(rr, (*rrx).Items...)
		pos = rrx.Position
		if pos == nil { break }
	}

	result := make([]Reflection, 0)
	for _, i := range rr {
		z := xlambda.GetFunctionConfigurationInput{
			FunctionName: i.Id,
		}
		rr, e := client.GetFunctionConfiguration(&z)
		if e != nil {
			// log.Println("getting function configuration: ", e, " (", *i.Id, ")")
		} else {
			/* "SQLCMD" is because that iswhat I use to deploy SQL */
			sql := rr.Environment.Variables["SQLCMD"]
			result = append(result, Reflection{Path: *i.Path, Name: *i.Id, SQL: *sql})
		}
	}

	sort.Sort(Nodes(result))

	a, err := json.MarshalIndent(result, "", " ")
	if err != nil {
		return sErr("unable to marshal list", err)
	}
	return events.APIGatewayProxyResponse{
		Body:       string(a),
		Headers:    map[string]string{},
		StatusCode: http.StatusOK,
	}, nil


}

func main() {
	/* for debugging */
	if len(os.Args) == 3 {
		file, _ := os.Open("deploy.json")
		defer file.Close()
		decoder := json.NewDecoder(file)
		conf := Configuration{}
		err := decoder.Decode(&conf)
		if err != nil {
			log.Fatal("unable to decode deploy.json: ", err)
		}

		// restApiID := os.Getenv("REST_API")
		// account := os.Getenv("AWS_ACCOUNT")
		// stage := os.Getenv("GATEWAY_STAGE")
		doReflect(os.Args[1], os.Args[2], conf.RestApiID, conf.Account, conf.Stage)
	} else {
		/* This is the actual invocation */
		lambda.Start(handleReflectRequest)
	}
}
