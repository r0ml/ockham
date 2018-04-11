#!/bin/sh -v

if [ -z "$1" ]; then
  echo "Forgot to provide the FUNCTION_NAME argument"
  exit 2
fi

echo "Compiling..."
GOOS=linux go build -o main
echo "Zipping..."
zip deployment.zip main 

GATEWAY_NAME=Hello
GATEWAY_PATH=clem

FUNCTION_NAME=$1
STAGE=prod

ACCT=`aws sts get-caller-identity --query Account --output text`
echo ACCT=$ACCT

REGION=`aws configure get region`
echo REGION=$REGION

LAMBDA_ROLE=`aws iam list-roles --query "Roles[?contains(RoleName,'lambda_basic')].Arn | [0]" --output text`
echo LAMBDA_ROLE=$LAMBDA_ROLE

echo lambda create-function
LAMBDA_ARN=`aws lambda create-function --function-name $FUNCTION_NAME --zip-file fileb://./deployment.zip --runtime go1.x --handler main --role $LAMBDA_ROLE --query "FunctionArn" --output text `
echo LAMBDA_ARN=$LAMBDA_ARN
if [ -z "$LAMBDA_ARN" ]; then
  LAMBDA_ARN=`aws lambda get-function-configuration --function-name $FUNCTION_NAME --query "FunctionArn" --output text `
  echo LAMBDA_ARN=$LAMBDA_ARN
fi

if [ -z "$LAMBDA_ARN" ]; then
  echo "Unable to create or find the function $FUNCTION_NAME"
  exit 2
fi


REST_API=`aws apigateway get-rest-apis --query "items[?name=='auther'].id" --output text`
if [ -z "$REST_API" ]; then
  echo apigateway create-rest-api
  REST_API=`aws apigateway create-rest-api --name $GATEWAY_NAME --query id --output text`
fi
echo REST_API=$REST_API

echo apigateway get-resources
PARENT_ID=`aws apigateway get-resources --rest-api-id $REST_API --query "items[?path=='/'].id" --output text `
echo PARENT_ID=$PARENT_ID

echo apigateway create-resource
RESOURCE_ID=`aws apigateway create-resource --rest-api-id $REST_API --parent-id $PARENT_ID --path-part $GATEWAY_PATH --query id --output text`
echo RESOURCE_ID=$RESOURCE_ID

echo apigateway put-method
aws apigateway put-method --rest-api-id $REST_API --resource-id $RESOURCE_ID --http-method ANY --authorization-type NONE

echo apigateway put-integration
aws apigateway put-integration --region $REGION --rest-api-id $REST_API --resource-id $RESOURCE_ID --http-method ANY --type AWS_PROXY --integration-http-method POST --uri arn:aws:apigateway:$REGION:lambda:path/2015-03-31/functions/$LAMBDA_ARN/invocations

echo apigateway create-deployment
aws apigateway create-deployment --rest-api-id $REST_API --stage-name $STAGE 

echo aws lambda add-permission
aws lambda add-permission --function-name $FUNCTION_NAME --statement-id apigateway-all-1 --action lambda:InvokeFunction --principal apigateway.amazonaws.com --source-arn "arn:aws:execute-api:$REGION:$ACCOUNT:$REST_API_ID/$STAGE/*/$GATEWAY_PATH" 


## test with:
# curl -X POST -d 'this is test data' https://$REST_API_ID.execute-api.$REGION.amazonaws.com/$STAGE/$GATEWAY_PATH
 
