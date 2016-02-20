// Copyright 2016 TANABE Ken-ichi. All rights reserved.
// Copyright 2015 Deniz Eren. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dynamostore

import (
	"bytes"
	"encoding/base32"
	"encoding/gob"
	"errors"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/nabeken/aws-go-dynamodb/attributes"
	"github.com/nabeken/aws-go-dynamodb/table"
	"github.com/nabeken/aws-go-dynamodb/table/option"
)

var (
	errSessionNotFound = errors.New("dynamostore: session data is not found")
	errSessionBroken   = errors.New("dynamostore: session data is broken")
)

var (
	// SessionIdHashKeyName is the name of attribute that represents session id.
	SessionIdHashKeyName = "session_id"

	// SessionDataKeyName is the name of attribute that represents session data encoded in Gob.
	SessionDataKeyName = "session_data"
)

// Store represents the session store backed by DynamoDB.
type Store struct {
	Table  *table.Table
	Codecs []securecookie.Codec

	// Option is served as the default configuration
	Options *sessions.Options
}

// New returns a new session store.
// See https://github.com/gorilla/sessions/blob/master/store.go for what keyPairs means.
// Especially for dynamostore, keyPairs is be used to authenticate session id.
// Yes, the actual data is stored in DynamoDB table.
func New(dynamodbAPI dynamodbiface.DynamoDBAPI, tableName string, keyPairs ...[]byte) *Store {
	// setting DynamoDB table wrapper
	t := table.New(dynamodbAPI, tableName).WithHashKey(SessionIdHashKeyName, "S")

	return &Store{
		Table:  t,
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
	}
}

// Get returns a session for the given name after adding it to the registry.
//
// See gorilla/sessions FilesystemStore.Get().
// or  boj/redistore
func (s *Store) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

func (s *Store) New(r *http.Request, name string) (*sessions.Session, error) {
	session := sessions.NewSession(s, name)

	// make a copy
	options := sessions.Options{}
	session.Options = &options
	session.IsNew = true
	if c, errCookie := r.Cookie(name); errCookie == nil {
		decodeErr := securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if decodeErr == nil {
			err := s.load(session)
			// mark as new if fails to load
			session.IsNew = err != nil
		}
	}

	return session, nil
}

// Save adds a single session to the response.
func (s *Store) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	// Marked for deletion
	if session.Options.MaxAge < 0 {
		s.delete(session)
		// Even we fail to delete, we should clear the cookie
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
		return nil
	}

	// Build an alphanumeric session id
	// FYI: Session ID is protected by MAC by securecookie so it can't be forged.
	if session.ID == "" {
		session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
	}
	if err := s.save(session); err != nil {
		return err
	}
	encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
	if err != nil {
		return err
	}
	http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

// save saves the session in DynamoDB table.
func (s *Store) save(session *sessions.Session) error {
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(session.Values); err != nil {
		return err
	}

	b := buf.Bytes()

	data := map[string]interface{}{
		SessionIdHashKeyName: session.ID,
		SessionDataKeyName:   b,
	}

	return s.Table.PutItem(data)
}

// load loads the session from dynamodb.
// returns error if session data does not exist in dynamodb
func (s *Store) load(session *sessions.Session) error {
	data := make(map[string]interface{})

	err := s.Table.GetItem(
		attributes.String(session.ID), nil,
		&data,
		option.ConsistentRead(),
	)
	if err != nil {
		return err
	}

	value, ok := data[SessionDataKeyName]
	if !ok {
		return errSessionNotFound
	}

	blob, ok := value.([]byte)
	if !ok {
		return errSessionBroken
	}

	return gob.NewDecoder(bytes.NewReader(blob)).Decode(&session.Values)
}

// delete deletes keys from DynamoDB table.
func (s *Store) delete(session *sessions.Session) error {
	return s.Table.DeleteItem(attributes.String(session.ID), nil)
}
