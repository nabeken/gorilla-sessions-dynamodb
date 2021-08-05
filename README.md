# gorilla-sessions-dynamodb

[![GoDoc](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](https://pkg.go.dev/github.com/nabeken/gorilla-sessions-dynamodb/dynamostore) [![Go](https://github.com/nabeken/gorilla-sessions-dynamodb/actions/workflows/go.yml/badge.svg)](https://github.com/nabeken/gorilla-sessions-dynamodb/actions/workflows/go.yml)

A session store backend for [gorilla/sessions](http://www.gorillatoolkit.org/pkg/sessions).

## Installation

```sh
go get -u github.com/nabeken/gorilla-sessions-dynamodb/dynamostore
```

## Documentation

Available on [pkg.go.dev](https://pkg.go.dev/github.com/nabeken/gorilla-sessions-dynamodb/dynamostore).

See http://www.gorillatoolkit.org/pkg/sessions for full documentation on underlying interface.

## Running integration tests

Before run the test, you should launch DynamoDBLocal:

```sh
docker run -d -p 8888:8000 amazon/dynamodb-local:1.13.6
```

then

```sh
cd dynamostore
export DYNAMOSTORE_DYNAMODB_ENDPOINT=http://127.0.0.1:8888
DYNAMOSTORE_INTEG_TEST=true go test -v
```

## DynamoDB table schema

You should create a table that has `session_id` as a HASH key.

You can still change the key name rather than `session_id`. Please consult the code.

# Acknowledgement and License

This package is a rewrite of [denizeren/dynamostore](https://github.com/denizeren/dynamostore)
to use [aws/aws-sdk-go](https://github.com/aws/aws-sdk-go).
