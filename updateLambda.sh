#!/bin/sh -x

set -e

function createFunction {
    if [[ ! -z $4 ]]; then
       VPC="--vpc-config $4"
    fi
    echo lambda create-function
    LAMBDA_ARN=`aws lambda create-function --function-name $2 --zip-file fileb://./$1.zip --runtime go1.x --handler main --role $3 $VPC --query "FunctionArn" --output text `
    echo Created LAMBDA_ARN=$LAMBDA_ARN
}

function updateFunction {
    echo -n "Compiling..."
    mkdir -p build
    FILENAME=$1
    FUNCTION_NAME=$2

    GOOS=linux go build -o build/main $FILENAME.go || { echo "go build failed"; exit 2; }

    echo -n "Zipping..."
    cp $FILENAME.go build/$FILENAME.go
    cd build
    zip $FILENAME.zip main $FILENAME.go $OTHERFILES

    ACCT=`aws sts get-caller-identity --query Account --output text`
    REGION=`aws configure get region`
    echo FUNCTION_NAME=$FUNCTION_NAME, ACCT=$ACCT, REGION=$REGION

    echo lambda update-function
    aws lambda update-function-code --function-name $FUNCTION_NAME --zip-file fileb://./$FILENAME.zip || createFunction $1 $2 $3 $4
    echo Done
}

case $1 in
deploy)
    FN=sql-deploy
    ROLE=`jq -r .LambdaDeployRole deploy.json`
    cp sql-proxy.zip build/sql.zip
    export OTHERFILES=sql.zip
    updateFunction deploy $FN $ROLE
    aws lambda update-function-configuration --function-name $FN --timeout 65
    ;;

login)
    FN=gatewayGoogleLogin
    ROLE=`jq -r .LambdaRole deploy.json`
    updateFunction login $FN $ROLE
    CLIENT_ID=`jq -r .GoogleWebClientID deploy.json`
    CLIENT_SECRET=`jq -r .GoogleWebClientSecret deploy.json`
    aws lambda update-function-configuration --function-name $FN --environment "Variables={CLIENT_ID=$CLIENT_ID,CLIENT_SECRET=$CLIENT_SECRET}"
    ;;
echo)
    ROLE=`jq -r .LambdaRole deploy.json`
    updateFunction echo echo $ROLE
    ;;
sql-proxy)
    ROLE=`jq -r .LambdaRole deploy.json`
    updateFunction proxy userProxy $ROLE
    ;;
sql)
    export AWS_PROFILE=`jq -r .RemoteProfile deploy.json`
    ROLE=`jq -r .RemoteLambdaRole deploy.json`
    VPC_CONFIG=`jq -r .RemoteVPC deploy.json`
    updateFunction sql sqlTemplate $ROLE $VPC_CONFIG
    ;;
sql-mole)
    export AWS_PROFILE=`jq -r .RemoteProfile deploy.json`
    ROLE=`jq -r .RemoteLambdaRole deploy.json`
    VPC_CONFIG=`jq -r .RemoteVPC deploy.json`
    updateFunction sql-mole sql-mole $ROLE $VPC_CONFIG
    ;;
sql-exec)
    export AWS_PROFILE=`jq -r .RemoteProfile deploy.json`
    ROLE=`jq -r .RemoteLambdaRole deploy.json`
    VPC_CONFIG=`jq -r .RemoteVPC deploy.json`
    updateFunction sql-exec sqlRun $ROLE $VPC_CONFIG
    ;;

http-proxy)
    ROLE=`jq -r .LambdaRole deploy.json`
    updateFunction http-proxy http-proxy $ROLE
    BUCKET=`jq -r .S3Bucket deploy.json`
    REMOTE_ROOT=`jq -r .RemoteRoot deploy.json`
    REMOTE_FUNCTION=`jq -r .RemoteHTTP deploy.json`
    aws lambda update-function-configuration --function-name http-proxy --environment "Variables={S3_BUCKET=$BUCKET,REMOTE_ROOT=$REMOTE_ROOT,REMOTE_FUNCTION=$REMOTE_FUNCTION}"
    ;;

http-mole)
    export AWS_PROFILE=`jq -r .RemoteProfile deploy.json`
    REMOTE_ROLE=`jq -r .RemoteLambdaRole deploy.json`
    VPC_CONFIG=`jq -r .RemoteVPC deploy.json`
    updateFunction http-mole http-mole $REMOTE_ROLE $VPC_CONFIG

    # AWS_PROFILE=warby aws lambda add-permission --function-name http-mole --action lambda:InvokeFunction --statement-id one --principal apigateway.amazonaws.com
    # AWS_PROFILE=warby aws lambda add-permission --function-name http-mole --action lambda:InvokeFunction --statement-id two --principal $THE_OTHER_ACCOUNT    ;;
    ;;

reflect)
    LDROLE=`jq -r .LambdaDeployRole deploy.json`
    updateFunction reflect reflect $LDROLE
    ;;
authorizer)
    REQDOMAIN=`jq -r .RequiredDomain deploy.json`
    ROLE=`jq -r .LambdaRole deploy.json`
    updateFunction authorizer googleOauthAuthorizer
    aws lambda update-function-configuration --function-name googleOauthAuthorizer --environment "Variables={ REQUIRED_DOMAIN=$REQDOMAIN }"
    ;;
*)
    echo "Unknown lambda for update:  must be one of login, echo, proxy, sql, authorizer"
    exit 2
esac
