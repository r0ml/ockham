package main

// for deployment, I need to set the REQUIRED_DOMAIN, CLIENT_ID and CLIENT_SECRET environment variables

import (
	"log"
	"errors"

	"os"
	"crypto/rand"
	"net/http"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"encoding/base64"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"golang.org/x/net/context"
	"io/ioutil"
	)

// TODO: This should be a specifiable option
// TODO: privateEcho is not a good redirect for the HTTP proxy
// TODO: possibly this can be taken from Stage Variables
var postLoginRedirect = "/../privateEcho"

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

type tuserinfo struct {
	Domain   string `json:"hd"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	GoogleID string `json:"sub"`
}

func tokenToJSON(token *oauth2.Token) (string, error) {
	if d, err := json.Marshal(token); err != nil {
		return "", err
	} else {
		print(string(d))
		return string(d), nil
	}
}

// also: https://www.googleapis.com/oauth2/v1/tokeninfo?access_token=

func main() {
	lambda.Start(handleLoginRequest)
}

func handleLoginRequest(ctx context.Context, r events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// this could possibly be saved in a global variable and reused within this lambda
	// this could possibly be saved in dynamodb?
	// Since this is a lambda, when the redirect comes back, I might not be here --
	// or the API Gateway might send the request to another instance.
	stateToken := "i wish this could be random"

	myClientId := os.Getenv("CLIENT_ID")
	mySecret := os.Getenv("CLIENT_SECRET")
	// meUrl := os.Getenv("LOGIN_URL")

	meUrl := r.Headers["X-Forwarded-Proto"]+"://"+r.Headers["Host"]+":"+r.Headers["X-Forwarded-Port"]+r.Path

	// this is using the "web flow" creds
	conf := &oauth2.Config{
		ClientID:     myClientId,
		ClientSecret: mySecret,

		RedirectURL: meUrl,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email", // You have to select your own scope from here -> https://developers.google.com/identity/protocols/googlescopes#google_sign-in
		},
		Endpoint: google.Endpoint,
	}

	url := conf.AuthCodeURL(stateToken)
	// log.Println("authURL: ", url)

	code := r.QueryStringParameters["code"]
	// log.Println("code: ", code)
	if len(code) == 0 {
		return events.APIGatewayProxyResponse{
			Body:       "",
			Headers:    map[string]string{"Location": url, "Cache-Control": "no-store"},
			StatusCode: http.StatusMovedPermanently,
		}, nil
	}

	state := r.QueryStringParameters["state"]

	if state != stateToken {
		log.Fatal("state doesn't match stateToken")
		os.Exit(2)
	}

	tok, err := conf.Exchange(context.TODO(), code)
	if err != nil {
		log.Fatal("exchanging context: ", err)
	}

	if !tok.Valid() {
		log.Fatal("retrieved invalid token")
		return events.APIGatewayProxyResponse{}, errors.New("retrieved invalid token")
	}

	client := conf.Client(context.TODO(), tok)

	email, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")

	if err != nil {
		log.Fatal("getting userinfo: ", err)
	}

	contents, err := ioutil.ReadAll(email.Body)
	log.Println(string(contents))

	var userinfo tuserinfo
	errx := json.Unmarshal(contents, &userinfo)
	if errx != nil {
		log.Fatal("unmarshaling: ",errx)
	}

	tj, err := tokenToJSON(tok)
	if err != nil {
		log.Fatal("couldn't serialize token", err)
	}

	tjx := base64.URLEncoding.EncodeToString([]byte(tj))

	// Do I need to 'jsonize' the token?
	dc := http.Cookie{
		Name:  "GoogleToken",
		Value: tjx,
		// FIXME:  shouldn't this be just for the subtree for this Gateway?
		Path:  "/",
		Secure: true,
		HttpOnly:  true,
		RawExpires: "0",
	}

	return events.APIGatewayProxyResponse{
		Body:       "login successful",
		Headers:    map[string]string{"Location": /* r.Path */ meUrl+postLoginRedirect, "Cache-Control": "no-store",
				"Set-Cookie": dc.String() },
		StatusCode: http.StatusMovedPermanently,
	}, nil

}
