// Package dynoutbox implements the transactional outbox pattern for Amazon DynamoDB.
//
// An application builds each event with NewEvent and saves it in the same DynamoDB
// transaction as its business change, so the event exists if and only if the change
// committed. A relay later publishes the saved events to Amazon EventBridge.
//
// The package is in early development and its API may change before v1.
package dynoutbox
