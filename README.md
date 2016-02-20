# gorilla-sessions-dynamodb

[![GoDoc](http://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](http://godoc.org/github.com/nabeken/gorilla-sessions-dynamodb/dynamostore) [![Build Status](https://travis-ci.org/nabeken/gorilla-sessions-dynamodb.svg?branch=master)](https://travis-ci.org/nabeken/gorilla-sessions-dynamodb)

A session store backend for [gorilla/sessions](http://www.gorillatoolkit.org/pkg/sessions).

## Installation

```sh
go get -u github.com/nabeken/gorilla-sessions-dynamodb/dynamostore
```

## Documentation

Available on [godoc.org](http://godoc.org/github.com/nabeken/gorilla-sessions-dynamodb/dynamostore).

See http://www.gorillatoolkit.org/pkg/sessions for full documentation on underlying interface.

## TODOs

- [ ] Add expiration date support
- [ ] Add max length support

# Acknowledgement and License

This package is a rewrite of [denizeren/dynamostore](https://github.com/denizeren/dynamostore)
to use [aws/aws-sdk-go](https://github.com/aws/aws-sdk-go).
