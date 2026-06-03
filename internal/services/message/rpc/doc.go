// Package rpc adapts message-service use cases to the JSON-RPC protocol.
//
// Protocol handlers validate request DTOs and call the app layer. They do not
// construct domain records directly.
package rpc
