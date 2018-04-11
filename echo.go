package main

import (
	"fmt"
	"context"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"encoding/json"
)

func handleEchoRequest(ctx context.Context, payload json.RawMessage) (events.APIGatewayProxyResponse, error) {
	res := string(payload)

	ctxy, _ := lambdacontext.FromContext(ctx)
	ctxz := fmt.Sprintf("%s\n\n%+v\n\n", res, ctxy)

	return events.APIGatewayProxyResponse{
		Body:       ctxz,
		Headers: map[string]string{"EchoHeader": "EchoValue",},
		StatusCode: http.StatusOK,
	}, nil
}

func main() {
	lambda.Start(handleEchoRequest)
}
