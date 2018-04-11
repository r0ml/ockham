package main

import (
	"log"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/apigateway"
	"os"
	"strings"
)

// TODO: This should be specified as a parameter or environment variable
var deploymentStage = "prod"

/*
restApiID string, account string, stage string, region string) (events.APIGatewayProxyResponse, error) {


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
log.Println("path: ",*i.Path)
for j := len(ss); j > rootDepth; j-- {
p := strings.Join(ss[0:j], "/")
if *i.Path == "/"+p {
// rootResource = "/" + p
rootDepth = j
pid = *i.Id
}
}
}

log.Println("starting")
for j := rootDepth; j < len(ss); j++ {
log.Println(restApiID, rootDepth, j)
log.Println(restApiID, pid, j, ss[j])
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

*/

func main() {
	name := os.Args[1]

	config := aws.Config{Region: aws.String("us-east-1") }

	sess, err := session.NewSession(&config )

	if err != nil {
		log.Fatal("getting session", err)
	}

	// client := xlambda.New(sess, &config)
	aclient := apigateway.New(sess, &config)
	// iclient := iam.New(sess, &config)

	gri := apigateway.GetRestApisInput{
		Limit:     aws.Int64(60),
	}


	rrx, e := aclient.GetRestApis(&gri)
	if e != nil {
		log.Fatal("couldn't get rest apis", e)
	}

	var id string;

	for _,v := range rrx.Items {
		if *v.Name == name {
			id = *v.Id
			break;
		}
	}
	log.Println("id", id)

	types := []string{"image/gif", "image/png", "image/jpg", "image/jpeg", "application/octet-stream", "application/x-font-woff" }
	pos := []*apigateway.PatchOperation{}

	for _,z := range types {
		po := apigateway.PatchOperation{
			Op:   aws.String(apigateway.OpAdd),
			Path: aws.String("/binaryMediaTypes/"+strings.Replace(z, "/", "~1", 1)),
		}
		pos = append(pos, &po)
	}

	uri := apigateway.UpdateRestApiInput{
		PatchOperations: pos,
		RestApiId: aws.String(id),
	}

	px, e := aclient.UpdateRestApi(&uri)
	log.Println(e, px)

	ux := apigateway.CreateDeploymentInput {
		RestApiId: aws.String(id),
		StageName: aws.String(deploymentStage),
		Description: aws.String("http-proxy-deploy command"),
	}

	xx, e := aclient.CreateDeployment(&ux)
	log.Println("create deployment: ", e, xx)

}