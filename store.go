package main

import (
	"os"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

func store(key string, value1 string, value2 string) *dynamodb.PutItemOutput {
	input := &dynamodb.PutItemInput{
		ConditionExpression: aws.String("attribute_not_exists(SessionId)"),
		Item: map[string]*dynamodb.AttributeValue{
			"SessionId": {
				S: aws.String(key),
			},
			"AccessToken": {
				S: aws.String(value1),
			},
			"RefreshToken": {
				S: aws.String(value2),
			},
			"Expires": {
				N: aws.String( fmt.Sprintf("%d", time.Now().Unix()+30) ),
			},
		},
		TableName: aws.String("oauth"),
	}
	result, err := svc.PutItem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				log.Println(dynamodb.ErrCodeConditionalCheckFailedException, aerr.Error())
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				log.Println(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				log.Println(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeItemCollectionSizeLimitExceededException:
				log.Println(dynamodb.ErrCodeItemCollectionSizeLimitExceededException, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				log.Println(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				log.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.Println(err.Error())
		}
		return nil
	}
	return result
}

func retrieve(key string) ([]string) {
	input := dynamodb.GetItemInput{
		Key:       map[string]*dynamodb.AttributeValue{"SessionId": {S: aws.String(key)}},
		TableName: aws.String("oauth"),
	}

	rx, err := svc.GetItem(&input)
	if err != nil {
		log.Fatal("getting item: ", err)
	}
	log.Println(*rx)

	itx := rx.Item
	at := itx["AccessToken"]
	rt := itx["RefreshToken"]
	if at == nil || rt == nil {
		return []string{}
	}
	return []string{*at.S,*rt.S}
}

func delete(key string) {
	input := dynamodb.DeleteItemInput{
		ConditionExpression: aws.String("attribute_exists(SessionId)"),
		Key:       map[string]*dynamodb.AttributeValue{"SessionId": {S: aws.String(key)}},
		TableName: aws.String("oauth"),
	}

	rx, err := svc.DeleteItem(&input)
	if err != nil {
		log.Fatal("getting item: ", err)
	}
	log.Println(rx)
}

func provision() {
	input := &dynamodb.CreateTableInput{
		TableName: aws.String("oauth"),
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{ AttributeName: aws.String("SessionId"), AttributeType: aws.String("S") },
	//		{ AttributeName: aws.String("AccessToken"), AttributeType: aws.String("S") },
	//		{ AttributeName: aws.String("RefreshToken"), AttributeType: aws.String("S") },
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{ AttributeName: aws.String("SessionId"), KeyType: aws.String("HASH"), },
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{ },
	}
	j, err := svc.CreateTable(input)
	if err != nil {
		log.Println("provisioning: ", err)
	}

	ip := &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String("oauth"),
		TimeToLiveSpecification: &dynamodb.TimeToLiveSpecification{
			AttributeName: aws.String("Expires"),
			Enabled: aws.Bool(true),
		},
	}

	res, err := svc.UpdateTimeToLive(ip)
	log.Printf("%+v\n, %+v\n", *j, *res)
}

var cfg *aws.Config
var sess *session.Session
var svc *dynamodb.DynamoDB

func main() {
	cfg = aws.NewConfig().WithRegion("us-east-1")
	sess, err := session.NewSession(cfg)
	if err != nil {
		log.Fatal("could not create AWS session")
	}
	// log.Println(*sess.Config.Region)

	svc = dynamodb.New(sess, cfg)

	switch len(os.Args) {
	case 1: // provision
		provision()
		log.Println("provisioned")
	case 2: // retrieve
		a := retrieve(os.Args[1])
		log.Printf("%+v\n", a)
	case 3: // delete?
		if os.Args[1] == "delete" {
			delete(os.Args[2])
			log.Printf("deleted")
			return
		}
		log.Fatal("only 'delete' implemented")
	case 4: // store
	   s := store(os.Args[1], os.Args[2], os.Args[3])
	   log.Printf("%+v\n", s)
	default:
		log.Fatal("not defined: 1 argument to retrieve, 3 arguments to store")
	}

}

