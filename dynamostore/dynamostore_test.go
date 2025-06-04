// Copyright (c) 2016 TANABE Ken-ichi
// Copyright (c) 2015 Deniz Eren
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dynamostore

import (
	"context"
	"encoding/gob"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gorilla/sessions"
)

type FlashMessage struct {
	Type    int
	Message string
}

func randSeq(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func newTestCreateTableInput(tableName string) *dynamodb.CreateTableInput {
	attributeName := aws.String(SessionIdHashKeyName)
	return &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: attributeName,
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: attributeName,
				KeyType:       types.KeyTypeHash,
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		TableName: aws.String(tableName),
	}
}

// newTestDynamoDBAPI returns a new instance of DynamoDB client but
// it points to DynamoDB Local endpoint instead of real endpoint.
func newTestDynamoDBAPI() (*dynamodb.Client, error) {
	endpoint := "http://127.0.0.1:8000"
	if ep := os.Getenv("DYNAMOSTORE_DYNAMODB_ENDPOINT"); ep != "" {
		endpoint = ep
	}

	// XXX: region is still need to set real one
	config, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("DUMMY", "DUMMY_SECRET_KEY", "DUMMY_TOKEN"),
		),
		config.WithRegion("eu-west-1"),
		config.WithBaseEndpoint(endpoint),
	)

	return dynamodb.NewFromConfig(config), err
}

// prepareDynamoDBTable prepares DynamoDB table and it returns table name.
func prepareDynamoDBTable(dynamodbClient *dynamodb.Client) string {
	dummyTableName := randSeq(10)

	input := newTestCreateTableInput(dummyTableName)
	dynamodbClient.CreateTable(context.TODO(), input)

	dynamodb.NewTableExistsWaiter(dynamodbClient).Wait(
		context.TODO(),
		&dynamodb.DescribeTableInput{
			TableName: aws.String(dummyTableName),
		},
		time.Minute,
	)

	return dummyTableName
}

func runIntegTest() bool {
	doIntegTest, _ := strconv.ParseBool(os.Getenv("DYNAMOSTORE_INTEG_TEST"))
	return doIntegTest
}

// extractCookie extract a cookie from response
func extractCookie(resp http.ResponseWriter) string {
	cookies := resp.Header()["Set-Cookie"]
	if len(cookies) == 0 {
		// test will be failed..
		return ""
	}
	return cookies[0]
}

func newTestRequestResponse() (*http.Request, *httptest.ResponseRecorder) {
	req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
	resp := httptest.NewRecorder()
	return req, resp
}

func TestUseSessionCookie(t *testing.T) {
	if !runIntegTest() {
		t.Skip("Do not run integration tests unless DYNAMOSTORE_INTEG_TEST is set")
	}

	dynamodbClient, err := newTestDynamoDBAPI()
	if err != nil {
		t.Fatalf("Error creating DynamoDB client: %v", err)
	}

	dummyTableName := prepareDynamoDBTable(dynamodbClient)
	defer dynamodbClient.DeleteTable(context.TODO(), &dynamodb.DeleteTableInput{
		TableName: aws.String(dummyTableName),
	})

	store := New(dynamodbClient, dummyTableName, []byte("secret-key"))
	store.UseSessionCookie = true

	req, resp := newTestRequestResponse()

	sessionKey := "cookie-session"

	// Get a session.
	_, err = store.Get(req, sessionKey)
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	// Save.
	err = sessions.Save(req, resp)
	if err != nil {
		t.Fatalf("Error saving session: %v", err)
	}

	// Eat cookie
	cookie := strings.ToLower(extractCookie(resp))
	if strings.Contains(cookie, "max-age") {
		t.Fatalf("Max-Age should not be in the cookie: got '%v'", cookie)
	}
	if strings.Contains(cookie, "expires") {
		t.Fatalf("Expires should not be in the cookie: got '%v'", cookie)
	}
}

func TestStoreExpiration(t *testing.T) {
	if !runIntegTest() {
		t.Skip("Do not run integration tests unless DYNAMOSTORE_INTEG_TEST is set")
	}

	var err error
	var session *sessions.Session

	sessionKey := "session-key-expires"

	dynamodbClient, err := newTestDynamoDBAPI()
	if err != nil {
		t.Fatalf("Error creating DynamoDB client: %v", err)
	}

	dummyTableName := prepareDynamoDBTable(dynamodbClient)
	defer dynamodbClient.DeleteTable(context.TODO(), &dynamodb.DeleteTableInput{
		TableName: aws.String(dummyTableName),
	})

	store := New(dynamodbClient, dummyTableName, []byte("secret-key"))

	// Set 3 seconds for max age
	store.MaxAge(3)

	req, resp := newTestRequestResponse()

	// Get a session.
	session, err = store.Get(req, sessionKey)
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	if !session.IsNew {
		t.Fatalf("Expected session.IsNew == true, Got: %v", session.IsNew)
	}

	// Add some flashes.
	session.AddFlash("foo")
	session.AddFlash("bar")

	// Save.
	err = sessions.Save(req, resp)
	if err != nil {
		t.Fatalf("Error saving session: %v", err)
	}

	// Eat cookie
	cookie := extractCookie(resp)
	lcCookie := strings.ToLower(cookie)
	if !strings.Contains(lcCookie, "max-age") {
		t.Fatalf("Max-Age should be in the cookie: got '%v'", cookie)
	}
	if !strings.Contains(lcCookie, "expires") {
		t.Fatalf("Expires should be in the cookie: got '%v'", cookie)
	}

	// Wait for 3 seconds
	time.Sleep(3 * time.Second)

	req, resp = newTestRequestResponse()
	req.Header.Add("Cookie", cookie)

	// Get a session.
	session, err = store.Get(req, sessionKey)
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	// session should be expired and it should be regenerated
	if !session.IsNew {
		t.Fatalf("Expected session.IsNew == true, Got: %v", session.IsNew)
	}

	// Check all saved values.
	flashes := session.Flashes()
	if len(flashes) != 0 {
		t.Fatalf("Expected empty flashes; Got %v", flashes)
	}
}

func TestStore(t *testing.T) {
	if !runIntegTest() {
		t.Skip("Do not run integration tests unless DYNAMOSTORE_INTEG_TEST is set")
	}

	var err error
	var session *sessions.Session

	sessionKey := "session-key"

	// Copyright 2012 The Gorilla Authors. All rights reserved.
	// Use of this source code is governed by a BSD-style
	// license that can be found in the LICENSE file.
	// https://github.com/gorilla/sessions/blob/master/sessions_test.go

	dynamodbClient, err := newTestDynamoDBAPI()
	if err != nil {
		t.Fatalf("Error creating DynamoDB client: %v", err)
	}

	dummyTableName := prepareDynamoDBTable(dynamodbClient)
	defer dynamodbClient.DeleteTable(context.TODO(), &dynamodb.DeleteTableInput{
		TableName: aws.String(dummyTableName),
	})

	store := New(dynamodbClient, dummyTableName, []byte("secret-key"))

	req, resp := newTestRequestResponse()

	// Get a session.
	session, err = store.Get(req, sessionKey)
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	// Get a flash.
	flashes := session.Flashes()
	if len(flashes) != 0 {
		t.Errorf("Expected empty flashes; Got %v", flashes)
	}

	// Add some flashes.
	session.AddFlash("foo")
	session.AddFlash("bar")

	// Custom key.
	session.AddFlash("baz", "custom_key")

	// Save.
	err = sessions.Save(req, resp)
	if err != nil {
		t.Fatalf("Error saving session: %v", err)
	}

	// Eat cookie
	cookie := extractCookie(resp)

	req, resp = newTestRequestResponse()
	req.Header.Add("Cookie", cookie)

	// Get a session.
	session, err = store.Get(req, sessionKey)
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	// Check all saved values.
	flashes = session.Flashes()
	if len(flashes) != 2 {
		t.Fatalf("Expected flashes; Got %v", flashes)
	}
	if flashes[0] != "foo" || flashes[1] != "bar" {
		t.Errorf("Expected foo,bar; Got %v", flashes)
	}

	// Flashes has been flushed so it will return nothing
	flashes = session.Flashes()
	if len(flashes) != 0 {
		t.Errorf("Expected dumped flashes; Got %v", flashes)
	}

	// Custom key.
	flashes = session.Flashes("custom_key")
	if len(flashes) != 1 {
		t.Errorf("Expected flashes; Got %v", flashes)

	} else if flashes[0] != "baz" {
		t.Errorf("Expected baz; Got %v", flashes)
	}

	// Flashes for custom key has been flushed so it will return nothing
	flashes = session.Flashes("custom_key")
	if len(flashes) != 0 {
		t.Errorf("Expected dumped flashes; Got %v", flashes)
	}

	// Save.
	session.Options.MaxAge = -1
	err = sessions.Save(req, resp)
	if err != nil {
		t.Fatalf("Error saving session: %v", err)
	}
}

func TestStore_CustomType(t *testing.T) {
	if !runIntegTest() {
		t.Skip("Do not run integration tests unless DYNAMOSTORE_INTEG_TEST is set")
	}

	var err error
	var session *sessions.Session

	sessionKey := "session-key"

	// Copyright 2012 The Gorilla Authors. All rights reserved.
	// Use of this source code is governed by a BSD-style
	// license that can be found in the LICENSE file.
	// https://github.com/gorilla/sessions/blob/master/sessions_test.go

	dynamodbClient, err := newTestDynamoDBAPI()
	if err != nil {
		t.Fatalf("Error creating DynamoDB client: %v", err)
	}

	dummyTableName := prepareDynamoDBTable(dynamodbClient)
	defer dynamodbClient.DeleteTable(context.TODO(), &dynamodb.DeleteTableInput{
		TableName: aws.String(dummyTableName),
	})

	store := New(dynamodbClient, dummyTableName, []byte("secret-key"))

	req, resp := newTestRequestResponse()

	// Get a session.
	session, err = store.Get(req, sessionKey)
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	// Get a flash.
	flashes := session.Flashes()
	if len(flashes) != 0 {
		t.Errorf("Expected empty flashes; Got %v", flashes)
	}

	// Add some flashes.
	session.AddFlash(&FlashMessage{42, "foo"})

	// Save.
	err = sessions.Save(req, resp)
	if err != nil {
		t.Fatalf("Error saving session: %v", err)
	}

	// Eat cookie
	cookie := extractCookie(resp)

	req, resp = newTestRequestResponse()
	req.Header.Add("Cookie", cookie)

	// Get a session.
	session, err = store.Get(req, sessionKey)
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}

	// Check all saved values.
	flashes = session.Flashes()
	if len(flashes) != 1 {
		t.Fatalf("Expected flashes; Got %v", flashes)
	}
	custom := flashes[0].(FlashMessage)
	if custom.Type != 42 || custom.Message != "foo" {
		t.Errorf("Expected %#v, got %#v", FlashMessage{42, "foo"}, custom)
	}

	// Delete session.
	session.Options.MaxAge = -1
	// Save.
	err = sessions.Save(req, resp)
	if err != nil {
		t.Fatalf("Error saving session: %v", err)
	}
}

func init() {
	gob.Register(FlashMessage{})
}
