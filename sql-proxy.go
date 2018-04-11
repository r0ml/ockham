package main

/* I need this as a zip file.

I get it as follows:
    echo -n "Compiling..."
    mkdir -p build
    FILENAME=sql-proxy
    GOOS=linux go build -o build/main $FILENAME.go || { echo "go build failed"; exit 2; }

    echo -n "Zipping..."
    cp $FILENAME.go build/$FILENAME.go
    cd build
    zip $FILENAME.zip main $FILENAME.go

 */

import (
	"context"
	"fmt"
	"os"
	"net/http"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	xlambda "github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-lambda-go/lambda"
)

func handleSQLProxyRequest(_ context.Context, resx events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	remoteFunction := os.Getenv("REMOTE_FUNCTION")
	connInfo := os.Getenv("CONNINFO")
	sqlCmd := os.Getenv("SQLCMD")

	resx.Headers["x-conninfo"] = connInfo
	resx.Headers["x-sqlcmd"] = sqlCmd

	payload, ee := json.Marshal(resx)
	if ee != nil {
		return events.APIGatewayProxyResponse{
			Body: fmt.Sprintf( "could not marshal proxy request: %v" , ee),
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	config := aws.Config{}

	sess, err := session.NewSession( &config )

	if err != nil {
		return events.APIGatewayProxyResponse{
			Body: fmt.Sprintf( "getting session: %v" , err),
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	client := xlambda.New(sess, &config )

	z := xlambda.InvokeInput{FunctionName: aws.String(remoteFunction), Payload: payload}

	result, err := client.Invoke(&z)

	var resy events.APIGatewayProxyResponse
	errx := json.Unmarshal(result.Payload, &resy)
	if errx != nil {
		return events.APIGatewayProxyResponse{
			Body: fmt.Sprintf("unmarshaling remote result: %v", errx),
			StatusCode: http.StatusInternalServerError,
		}, nil
	}

	return events.APIGatewayProxyResponse{
		Body:       resy.Body,
		Headers:    resy.Headers,
		StatusCode: resy.StatusCode,
	}, nil
}

/** TODO:  Called from the command line, this could package itself up and write itself to S3 */
func main() {
	lambda.Start(handleSQLProxyRequest)
}
