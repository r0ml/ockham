package main

/* This is the lambda to implement a custom authorizer that uses Google Authentication */
import (
	"context"
	"net/http"
	"os"
	"io/ioutil"
	"encoding/json"
	"encoding/base64"

	"golang.org/x/oauth2"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

// Helper function to generate an IAM policy
func generatePolicy(principalId, effect, resource string) events.APIGatewayCustomAuthorizerResponse {
	authResponse := events.APIGatewayCustomAuthorizerResponse{PrincipalID: principalId}

	if effect != "" && resource != "" {
		authResponse.PolicyDocument = events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   effect,
					Resource: []string{resource},
				},
			},
		}
	}

	// Optional output with custom properties of the String, Number or Boolean type.
	authResponse.Context = map[string]interface{}{
		"stringKey":  "stringval",
		"numberKey":  123,
		"booleanKey": true,
	}
	return authResponse
}

/* Recover the oauth2 token from a JSON string */
func tokenFromJSON(jsonStr string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

/* This structure should recover the Google User info from a JSON object */
type Userinfo struct {
	Domain string `json:"hd"`
	Email string `json:"email"`
	Name string `json:"name"`
	GoogleID string `json:"sub"`
}

/*type APIGatewayCustomAuthorizerRequest struct {
	Type string `json:"type"`
	MethodArn string `json:"methodArn"`
	Resource string `json:"resource"`
	Path string `json:"Path"`
	HttpMethod string `json:"httpMethod"`
	Headers map[string]string `json:"headers"`
	QueryStringParameters map[string]string `json:"queryStringParameters"`
	PathParameters map[string]string `json:"pathParameters"`
	StageVariables map[string]string `json:"stageVariables"`
	RequestContext events.APIGatewayProxyRequestContext
}*/

/* Handle the authorization request */
func handleRequest(ctx context.Context, eventa json.RawMessage) (events.APIGatewayCustomAuthorizerResponse, error) {
	var event events.APIGatewayCustomAuthorizerRequest
	var eventb events.APIGatewayProxyRequest

	fail := generatePolicy("error", "Deny", event.MethodArn)

	// I'm going to unmarshal the argument twice:  Once to get the Custom Authorizer Request,
	// and once to get the API Gateway Proxy Request */
	err := json.Unmarshal(eventa, &event)
	errb := json.Unmarshal(eventa, &eventb)

	if err != nil {
		return fail, err
	}

	if errb != nil {
		return fail, errb
	}

	var token string
	if event.Type == "TOKEN" {
		// log.Println("TOKEN")
		token = event.AuthorizationToken
	} else if event.Type == "REQUEST" {
		// log.Println("REQUEST")
		token = eventb.Headers["authorization"]
		if token == "" {
			ck := eventb.Headers["cookie"]

			// fake out the machinery to parse the cookies
			header := http.Header{}
			header.Add("Cookie", ck)
			request := http.Request{Header: header}
			ckx := request.Cookies()
			// log.Println("Got Cookies")
			for _, c := range ckx {
				if c.Name == "GoogleToken" {
					tkj := c.Value
					t, errm := base64.URLEncoding.DecodeString(string(tkj))
					if errm == nil {
						tt, errn := tokenFromJSON(string(t))
						if errn == nil {
							// log.Println("Got bearer token: ",tt.AccessToken)
							token = tt.AccessToken
							token = "Bearer " + token
						}
					}
				}
			}
		}
	}

	if token == "" {
		// log.Println("Generating empty token")
		return generatePolicy("unknown", "Deny", event.MethodArn), nil
	}

	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	req.Header.Add("Authorization", token)

	client := &http.Client{}

	email, err := client.Do(req)

	// log.Println("result of Do: ", err, email)

	if err != nil {
		return fail, err
	}

	contents, err := ioutil.ReadAll(email.Body)

	// log.Println(" results of readAll: ", contents, err)
	if err != nil {
		return fail, err
	}

	var userinfo Userinfo
	errx := json.Unmarshal(contents, &userinfo)

	// log.Println("results of Unmarshal: ", errx, userinfo)
	if errx != nil {
		return fail, errx
	}

	if userinfo.Domain == os.Getenv("REQUIRED_DOMAIN")  {
		// log.Println("validated; will allow")
		return generatePolicy(userinfo.Email, "Allow", event.MethodArn), nil
	} else {
		// log.Println("not required domain")
		return generatePolicy(userinfo.Email, "Deny", event.MethodArn), nil
	}
}

func main() {
	lambda.Start(handleRequest)
}
