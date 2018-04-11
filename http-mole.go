package main

import (
	"fmt"
	"log"
	"context"
	"net/http"
	"net/url"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"strings"
	"io/ioutil"
	"os"
	"encoding/base64"
)

func handleHTTPRequest(_ context.Context, payload events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	path := payload.PathParameters["proxy"]
	// log.Println(path)

	urlx := os.Getenv("REMOTE_ROOT")+path

	var qp string = ""

	xurl, ok := payload.Headers["x-remote-root"]
	if ok {
		urlx = xurl + path
	}

	for k,v := range payload.QueryStringParameters {
		qp += "&"+k+"="+url.QueryEscape(v)
	}
	if len(qp)>0 {
		urlx += "?" + qp[1:]
	}


	req, _ := http.NewRequest(payload.HTTPMethod, urlx, strings.NewReader(payload.Body))

	// log.Println("request headers", payload.Headers)
	/*for k,v := range payload.Headers {
		req.Header.Set(k,v)
	}*/

	ctx := payload.Headers["content-type"]

	req.Header.Set("host", req.Host)
	if ctx != "" { req.Header.Set("content-type", ctx) }
	vv := payload.Headers["cookie"]; if vv != "" { req.Header.Set("cookie", vv) }
	vv = payload.Headers["accept"]; if vv != "" { req.Header.Set( "accept", vv ) }

	// log.Println("method", payload.HTTPMethod)
	// log.Println("url",urlx)
	// log.Println("sent request headers", req.Header)
	// log.Println("body",payload.Body)

	c := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := c.Do(req)
	if err != nil {
		log.Fatal("http Do: ", err)
	}

	// log.Println("response headers", resp.Header)

	resx, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("http", err)
	}

	var s string

	// log.Println("length of resx: ", len(resx))
	ct := resp.Header.Get("content-type")

	// TODO:  I should read the binary media types from the Gateway configuration and use that list here
	isbin := (map[string]int{"image/png":1, "image/jpeg":1, "image/jpg":1, "application/octet-stream":1,
		"application/x-font-woff":1 })[ct] == 1

	// log.Println("isbin: ", ct, isbin)

	if isbin {
		s = base64.StdEncoding.EncodeToString(resx)
		// log.Println("length of encoded resx: ", len(s))
	} else {
		s = string(resx)
		// log.Println("length of stringified resx: ", len(s))
	}

	h := make(map[string]string, 0)

	v := resp.Header.Get("content-type"); if v != "" { h["Content-Type"] = v }
	v = resp.Header.Get("set-cookie"); if v != "" { h["Set-Cookie"] = v }
	v = resp.Header.Get("strict-transport-security"); if v != "" { h["Strict-Transport-Security"] = v }
	v = resp.Header.Get("location"); if v != "" { h["Location"] = v }

	// also pass along:
	// last-modified, expires, etag, cache-control

	// log.Println("response headers sent", h)

	z := events.APIGatewayProxyResponse{
		Body:       s,
		Headers:    h,
		StatusCode: resp.StatusCode,
		IsBase64Encoded: isbin,
	}
	return z, nil
}

func errResponse(s string, err error) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse {
		Body: fmt.Sprintf("%s: %s", s, err),
		StatusCode: http.StatusBadRequest,
	}, nil
}

func main() {
	lambda.Start(handleHTTPRequest)
}
