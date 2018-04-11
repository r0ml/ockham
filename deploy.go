package main

import (
	"fmt"
	"context"
	"os"
	"log"
	"io/ioutil"
	"strings"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	xlambda "github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"github.com/aws/aws-sdk-go/aws/session"
)

/* This could deploy either:
    a) A "proxy" lambda which calls the gateway lambda or
	b) A "SQL" lambda on the warby parker VPN
 */

func handleDeployRequest(_ context.Context, r events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// TODO: How can I figure out what the deployment region is?
	// i.e. -- what region am I running in?
	restApiId := r.RequestContext.APIID
	account := r.RequestContext.AccountID
	stage := r.RequestContext.Stage

	vs, e := url.ParseQuery(string( r.Body))

	if e != nil {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("%v", e),
			StatusCode: 502,
		}, nil
	}

	cmd := vs.Get("command")
	gatewayPath := vs.Get("path")


	// Copied from the command line version
	// this presumes that I have deployed "deploy.json" in the deployer lambda
	// TODO: Instead of generating a json file, why not generate a Go file that has the conf baked in?
	file, _ := os.Open("deploy.json")
	defer file.Close()
	decoder := json.NewDecoder(file)
	conf := Configuration{}
	err := decoder.Decode(&conf)
	if err != nil {
		log.Fatal("unable to decode deploy.json: ", err)
	}






	g, e := doDeploy(gatewayPath, cmd, restApiId, account, stage, conf.Region,
		conf.AuthorizerID, conf.RemoteSQL, conf.LambdaRole, conf.ConnectionInfo)
	return g,nil
}

func sendError(m string, err error) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		Body:       fmt.Sprintf("%s: %s", m, err),
		Headers: map[string]string{},
		StatusCode: http.StatusBadRequest,
	}, err
}

func doDeploy(gatewayPath string, cmd string,
	restApiID string, account string, stage string, region string, authorizerID string,
		remoteFunctionARN string, lambdaIAMRole string,
			connectionInfo string) (events.APIGatewayProxyResponse, error) {

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)

	if err != nil {
		return sendError("getting session", err)
	}

	client := xlambda.New(sess, &aws.Config{Region: aws.String(region)})
	aclient := apigateway.New(sess, &aws.Config{Region: aws.String(region)})
	iclient := iam.New(sess, &aws.Config{Region: aws.String(region)})

	// log.Printf("invoke input: %s\n\n%+v\n\n", remoteFunction, string(payload) )
	ss := strings.Split(gatewayPath, "/")
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
			return sendError("couldn't get resources", e)
		}
		rr = append(rr, (*rrx).Items...)
		pos = rrx.Position
		if pos == nil { break }
	}

	// var rootResource string = ""
	var rootDepth = -1
	var pid string

	for _, i := range rr {
		// log.Println("path: ",*i.Path)
		for j := len(ss); j > rootDepth; j-- {
			p := strings.Join(ss[0:j], "/")
			if *i.Path == "/"+p {
				// rootResource = "/" + p
				rootDepth = j
				pid = *i.Id
			}
		}
	}

	// log.Println("starting")
	for j := rootDepth; j < len(ss); j++ {
		// log.Println(restApiID, rootDepth, j)
		// log.Println(restApiID, pid, j, ss[j])
		cri := apigateway.CreateResourceInput{
			RestApiId: aws.String(restApiID),
			ParentId:  aws.String(pid),
			PathPart:  aws.String(ss[j]),
		}
		a, e := aclient.CreateResource(&cri)
		if e != nil {
			return sendError("did not create resource", e)
		}
		pid = *a.Id
	}

	// log.Println("good to go")

	// Now pid is the final ID for the last resource created

	// TODO: Instead of loading the function code via sql.zip, I should point this at an S3 object
	// TODO: the variables (CONNINFO, REMOTE_FUNCTION, and authorizerID) should be taken from a config file in sql.zip?
	zfb, err := ioutil.ReadFile("sql.zip")
	if err != nil {
		return sendError("couldn't read sql.zip", err)
	}

	var v = make(map[string]*string)

	v["SQLCMD"] = aws.String(cmd)
	v["CONNINFO"] = aws.String(connectionInfo)
	v["REMOTE_FUNCTION"] = aws.String(remoteFunctionARN)

	env := xlambda.Environment{
		Variables: v,
	}

	grli := iam.GetRoleInput {
		RoleName: aws.String(lambdaIAMRole),
	}
	rl, e := iclient.GetRole(&grli)
	if err != nil {
		return sendError("couldn't get role", e)
	}
	// log.Println(rl)


	code := xlambda.FunctionCode{
		ZipFile: zfb,
	}

	functionName := pid
	var farn string

	if rootDepth == len(ss) {
		// In this case, I assume that I'm updating an existing function, since the gateway route was
		// already defined

		z := xlambda.UpdateFunctionCodeInput{
			FunctionName:aws.String(functionName),
			ZipFile: zfb,
		}
		result, err := client.UpdateFunctionCode(&z)
		if err != nil {
			return sendError("did not update function code", err)
		}
		farn = *result.FunctionArn

		zz := xlambda.UpdateFunctionConfigurationInput{
			FunctionName: aws.String(functionName),
			Environment: &env,
			Role: rl.Role.Arn,
		}
		_, errx := client.UpdateFunctionConfiguration(&zz)
		if errx != nil {
			return sendError("did not update function configuration", errx)
		}
		// log.Println(result, "\n\n", resultx)
	} else {

		z := xlambda.CreateFunctionInput{
			FunctionName: aws.String(functionName),
			Code:         &code,
			Role:         rl.Role.Arn,
			Environment:  &env,
			Runtime:      aws.String("go1.x"),
			Handler:      aws.String("main"),
		}

		result, err := client.CreateFunction(&z)
		if err != nil {
			return sendError("did not create function", err)
		}

		farn = *result.FunctionArn

		// log.Println(result)

	}

	// If the provided gateway path is multi-level, then I need to create or get the id of
	// every resource down the path chain.
	// The final part is what gets created and hooked up to the function

	authId := aws.String(authorizerID)

	pmi := apigateway.PutMethodInput{
		RestApiId:         aws.String(restApiID),
		ResourceId:        aws.String(pid),
		HttpMethod:        aws.String("ANY"),
		AuthorizationType: aws.String("CUSTOM"),
		AuthorizerId:      authId,
	}
	_, e = aclient.PutMethod(&pmi)
	if e != nil {
		log.Println("did not put method: ", e)
	}

	pii := apigateway.PutIntegrationInput{
		RestApiId:             aws.String(restApiID),
		ResourceId:            aws.String(pid),
		HttpMethod:            aws.String("ANY"),
		Type:                  aws.String("AWS_PROXY"),
		IntegrationHttpMethod: aws.String("POST"),
		Uri:                   aws.String("arn:aws:apigateway:" + region + ":lambda:path/2015-03-31/functions/" + farn + "/invocations"),
	}
	_, e = aclient.PutIntegration(&pii)
	// Actually, if this fails, it is OK :  the lambda function name is the resource id -- so I can assume
	// I did it once before
	if e != nil {
		log.Println("did not put integration: ", e)
	}

	api := xlambda.AddPermissionInput{
		FunctionName: aws.String(functionName),
		StatementId:  aws.String("apigateway-" + pid),
		Action:       aws.String("lambda:InvokeFunction"),
		Principal:    aws.String("apigateway.amazonaws.com"),
		SourceArn:    aws.String("arn:aws:execute-api:" + region + ":" + account + ":" + restApiID + "/" + stage + "/*/" + gatewayPath),
	}
	_, e = client.AddPermission(&api)
	if e != nil {
		log.Println("did not add permission: ", e)
	}

	did := "none (function updated)"
	log.Printf("rootDepth: %d, len(ss): %d", rootDepth, len(ss))
	if rootDepth < len(ss) {
		// log.Println("deploying")
		adi := apigateway.CreateDeploymentInput{
			RestApiId: aws.String(restApiID),
			StageName: aws.String("prod"),
		}

		d, e := aclient.CreateDeployment(&adi)
		if e != nil {
			return sendError("couldnt create deployment", e)
		}
		did = *d.Id
	}

	msg := "You're good to go"
	if rootDepth < len(ss) {
		msg = "Wait 60 seconds and this resource will appear"
	}
	return events.APIGatewayProxyResponse{
		Body:       fmt.Sprintf("Deployment: %s, FunctionName: %s\n\n%s", did, functionName, msg),
		Headers:    map[string]string{},
		StatusCode: http.StatusOK,
	}, nil


}

type Configuration struct {
	RestApiID string
	Account string
	Stage string
	Region string
	AuthorizerID string
	RemoteSQL string
	LambdaRole string
	ConnectionInfo string
}


func main() {
	if len(os.Args) == 3 {
		file, _ := os.Open("deploy.json")
		defer file.Close()
		decoder := json.NewDecoder(file)
		conf := Configuration{}
		err := decoder.Decode(&conf)
		if err != nil {
			log.Fatal("unable to decode deploy.json: ", err)
		}

		doDeploy(os.Args[1], os.Args[2], conf.RestApiID, conf.Account, conf.Stage, conf.Region,
			conf.AuthorizerID, conf.RemoteSQL, conf.LambdaRole, conf.ConnectionInfo)

	} else {
		lambda.Start(handleDeployRequest)
	}
}
