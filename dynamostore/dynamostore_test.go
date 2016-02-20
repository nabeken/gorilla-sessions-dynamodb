// Copyright (c) 2015 Deniz Eren
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dynamostore

import (
	"encoding/gob"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
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
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: attributeName,
				AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: attributeName,
				KeyType:       aws.String(dynamodb.KeyTypeHash),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		TableName: aws.String(tableName),
	}
}

// newTestDynamoDBAPI returns a new instance of DynamoDB client but
// it points to DynamoDB Local endpoint instead of real endpoint.
func newTestDynamoDBAPI() *dynamodb.DynamoDB {
	endpoint := "http://127.0.0.1:8000"
	if ep := os.Getenv("DYNAMOSTORE_DYNAMODB_ENDPOINT"); ep != "" {
		endpoint = ep
	}

	// XXX: region is still need to set real one
	c := aws.NewConfig().
		WithRegion("eu-west-1").
		WithEndpoint(endpoint).
		WithCredentials(credentials.NewStaticCredentials("DUMMY", "DUMMY_SECRET_KEY", "DUMMY_TOKEN"))

	return dynamodb.New(session.New(c))
}

func TestStore(t *testing.T) {
	if doIntegTest, _ := strconv.ParseBool(os.Getenv("DYNAMOSTORE_INTEG_TEST")); !doIntegTest {
		t.Skip("Do not run integration tests unless DYNAMOSTORE_INTEG_TEST is set")
	}

	var err error
	var ok bool
	var cookies []string
	var session *sessions.Session

	// Copyright 2012 The Gorilla Authors. All rights reserved.
	// Use of this source code is governed by a BSD-style
	// license that can be found in the LICENSE file.

	dynamodbClient := newTestDynamoDBAPI()

	dummyTableName := randSeq(10)

	input := newTestCreateTableInput(dummyTableName)
	dynamodbClient.CreateTable(input)

	dynamodbClient.WaitUntilTableExists(&dynamodb.DescribeTableInput{
		TableName: aws.String(dummyTableName),
	})

	defer dynamodbClient.DeleteTable(&dynamodb.DeleteTableInput{
		TableName: aws.String(dummyTableName),
	})

	store := New(dynamodbClient, dummyTableName, []byte("secret-key"))

	// Round 1 ----------------------------------------------------------------
	{
		req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
		rsp := httptest.NewRecorder()
		// Get a session.
		if session, err = store.Get(req, "session-key"); err != nil {
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
		if err = sessions.Save(req, rsp); err != nil {
			t.Fatalf("Error saving session: %v", err)
		}
		hdr := rsp.Header()
		cookies, ok = hdr["Set-Cookie"]
		if !ok || len(cookies) != 1 {
			t.Fatalf("No cookies. Header:", hdr)
		}
	}

	// Round 2 ----------------------------------------------------------------
	{
		req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
		req.Header.Add("Cookie", cookies[0])
		rsp := httptest.NewRecorder()
		// Get a session.
		if session, err = store.Get(req, "session-key"); err != nil {
			t.Fatalf("Error getting session: %v", err)
		}
		// Check all saved values.
		flashes := session.Flashes()
		if len(flashes) != 2 {
			t.Fatalf("Expected flashes; Got %v", flashes)
		}
		if flashes[0] != "foo" || flashes[1] != "bar" {
			t.Errorf("Expected foo,bar; Got %v", flashes)
		}
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
		flashes = session.Flashes("custom_key")
		if len(flashes) != 0 {
			t.Errorf("Expected dumped flashes; Got %v", flashes)
		}

		session.Options.MaxAge = -1
		// Save.
		if err = sessions.Save(req, rsp); err != nil {
			t.Fatalf("Error saving session: %v", err)
		}
	}

	// Round 3 ----------------------------------------------------------------
	// Custom type
	{
		req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
		rsp := httptest.NewRecorder()
		// Get a session.
		if session, err = store.Get(req, "session-key"); err != nil {
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
		if err = sessions.Save(req, rsp); err != nil {
			t.Fatalf("Error saving session: %v", err)
		}
		hdr := rsp.Header()
		cookies, ok = hdr["Set-Cookie"]
		if !ok || len(cookies) != 1 {
			t.Fatalf("No cookies. Header:", hdr)
		}
	}

	// Round 4 ----------------------------------------------------------------
	// Custom type
	{
		req, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
		req.Header.Add("Cookie", cookies[0])
		rsp := httptest.NewRecorder()
		// Get a session.
		if session, err = store.Get(req, "session-key"); err != nil {
			t.Fatalf("Error getting session: %v", err)
		}

		// Check all saved values.
		flashes := session.Flashes()
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
		if err = sessions.Save(req, rsp); err != nil {
			t.Fatalf("Error saving session: %v", err)
		}
	}
}

func init() {
	gob.Register(FlashMessage{})
}
