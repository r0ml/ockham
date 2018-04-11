package main

import (
	"context"
	"fmt"
	"os"
	"net/http"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	xlambda "github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/s3"
	"log"
	"io/ioutil"
	"encoding/base64"
)

func handleHTTPProxyRequest(_ context.Context, resx events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
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

	// First, we'll go for the local S3 file
	sroot := os.Getenv("S3_BUCKET")
	if sroot != "" {
		log.Println("get object ", sroot, resx.Path)
		sclient := s3.New(sess, &config)
		goi := s3.GetObjectInput{
			Bucket: aws.String(sroot),
			Key:    aws.String(resx.Path),
		}

		y, err := sclient.GetObject(&goi)
		if err == nil {
			/* if not error, return it.  Otherwise, try the http proxy */
			t, errz := ioutil.ReadAll(y.Body)
			if errz != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("reading object body: %v", err),
					StatusCode: http.StatusInternalServerError,
				}, nil
			}

			ct := *y.ContentType
			// TODO:  I should read the binary media types from the Gateway configuration and use that list here
			isbin := (map[string]int{"image/png": 1, "image/jpeg": 1, "image/jpg": 1, "application/octet-stream": 1,
				"application/x-font-woff": 1})[ct] == 1

			// log.Println("isbin: ", ct, isbin)
			var s string
			if isbin {
				s = base64.StdEncoding.EncodeToString(t)
				// log.Println("length of encoded resx: ", len(s))
			} else {
				s = string(t)
				// log.Println("length of stringified resx: ", len(s))
			}

			ress := events.APIGatewayProxyResponse{
				Body:            s,
				Headers:         map[string]string{"content-type" : ct},
				StatusCode:      http.StatusOK,
				IsBase64Encoded: isbin,
			}
			return ress, nil
		}

		log.Println("s3 says: ", err)
	}

	rroot := os.Getenv("REMOTE_ROOT")
	remoteFunction := os.Getenv("REMOTE_FUNCTION")
	if rroot != "" {
		resx.Headers["x-remote-root"] = rroot
		client := xlambda.New(sess, &config)

		z := xlambda.InvokeInput{FunctionName: aws.String(remoteFunction), Payload: payload}

		result, err := client.Invoke(&z)
		if err != nil {
			return events.APIGatewayProxyResponse{
				Body: fmt.Sprintf( "invoking remote http lambda: %v" , err),
				StatusCode: http.StatusInternalServerError,
			}, nil
		}

		// log.Println( string(string(result.Payload)) )

		var resy events.APIGatewayProxyResponse

		errx := json.Unmarshal(result.Payload, &resy)
		if errx != nil {
			return events.APIGatewayProxyResponse{
				Body:       fmt.Sprintf("unmarshaling remote result: %v", errx),
				StatusCode: http.StatusInternalServerError,
			}, nil
		}

		// log.Println("length of body", len(resy.Body), resy.IsBase64Encoded )

		ress := events.APIGatewayProxyResponse{
			Body:            resy.Body,
			Headers:         resy.Headers,
			StatusCode:      resy.StatusCode,
			IsBase64Encoded: bool(resy.IsBase64Encoded),
		}

		// log.Println("the request", resx)
		// log.Println("the response", resy)
		// log.Println("got headers", resy.Headers)
		// log.Println("passed on headers", ress.Headers, ress.IsBase64Encoded)

		return ress, nil
	}
	return events.APIGatewayProxyResponse{
		Body:       "did not find either s3 object or remote http",
		StatusCode: http.StatusInternalServerError,
	}, nil

}

func main() {
	lambda.Start(handleHTTPProxyRequest)
}
