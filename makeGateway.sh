#!/bin/sh -v

if [ -z "$1" ]; then
  echo "Forgot to provide the GATEWAY_NAME argument"
  exit 2
fi

GATEWAY_NAME=$1

FUNCTION_NAME=testAuthorizer

ACCT=`aws sts get-caller-identity --query Account --output text`
echo ACCT=$ACCT

REGION=`aws configure get region`
echo REGION=$REGION

echo apigateway create-rest-api
REST_API=`aws apigateway create-rest-api --name $GATEWAY_NAME --description "A gateway for generated and managed lambdas" --query id --output text`

# probably want to add --binary-media-types ?

echo REST_API=$REST_API


# this gets the parent id for creating resources
echo apigateway get-resources
PARENT_ID=`aws apigateway get-resources --rest-api-id $REST_API --query "items[?path=='/'].id" --output text `
echo PARENT_ID=$PARENT_ID


### Set up the echo function
### this presupposes that an echo function has previously been created

function makeResource {
  MR_PARENT_ID=$1
MR_GATEWAY_PATH=$2
MR_FUNCTION_NAME=$3
echo apigateway create-resource

MR_RESOURCE_ID=`aws apigateway create-resource --rest-api-id $REST_API --parent-id $MR_PARENT_ID --path-part $MR_GATEWAY_PATH --query id --output text`
echo MR_RESOURCE_ID=$MR_RESOURCE_ID

echo apigateway put-method
aws apigateway put-method --rest-api-id $REST_API --resource-id $MR_RESOURCE_ID --http-method ANY --authorization-type NONE

MR_LAMBDA_ARN=`aws lambda get-function-configuration --function-name $MR_FUNCTION_NAME --query "FunctionArn" --output text `
echo MR_LAMBDA_ARN=$MR_LAMBDA_ARN

echo apigateway put-integration
aws apigateway put-integration --region $REGION --rest-api-id $REST_API --resource-id $MR_RESOURCE_ID --http-method ANY --type AWS_PROXY --integration-http-method POST --uri arn:aws:apigateway:$REGION:lambda:path/2015-03-31/functions/$MR_LAMBDA_ARN/invocations

echo aws lambda add-permission
aws lambda add-permission --function-name $MR_FUNCTION_NAME  --statement-id apigateway-$MR_GATEWAY_PATH --action lambda:InvokeFunction --principal apigateway.amazonaws.com --source-arn "arn:aws:execute-api:$REGION:$ACCOUNT:$REST_API/$STAGE/*/$MR_GATEWAY_PATH" 
}

makeResource $PARENT_ID echo echo








## Need methods before I can create a deployment
# aws apigateway create-deployment --rest-api-id $REST_API --stage-name dev --stage-description 'Development Stage' --description 'First deployment to the dev stage'

LAMBDA_ARN=`aws lambda get-function-configuration --function-name $FUNCTION_NAME --query "FunctionArn" --output text `

AUTH_ID=`aws apigateway create-authorizer --rest-api-id $REST_API --name 'Google_Custom_Authorizer' --type TOKEN --authorizer-uri arn:aws:apigateway:$REGION:lambda:path/2015-03-31/functions/$LAMBDA_ARN/invocations --identity-source 'method.request.header.Authorization' --authorizer-result-ttl-in-seconds 300 --query id --output text`
echo AUTH_ID=$AUTH_ID

aws lambda add-permission --function-name $LAMBDA_ARN --action lambda:InvokeFunction --statement-id $AUTH_ID --principal apigateway.amazonaws.com --source-arn arn:aws:execute-api:$REGION:$ACCT:$REST_API/authorizers/$AUTH_ID

#########
## must remember to DEPLOY the api whenever things are changed.

