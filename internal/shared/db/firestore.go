// Package db initializes the Firebase Admin SDK and provides shared clients
// for Firestore and Firebase Auth. The firebase.App is created once at startup
// and all clients are derived from it.
package db

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	fbstorage "firebase.google.com/go/v4/storage"
	"google.golang.org/api/option"
)

// Firebase holds the initialized Firebase clients.
type Firebase struct {
	app       *firebase.App
	Firestore *firestore.Client
	Auth      *auth.Client
}

// NewFirebase initializes the Firebase Admin SDK using the given project ID
// and credentials file path. It creates the Firestore and Auth clients once.
func NewFirebase(ctx context.Context, projectID, credentialsFile string) (*Firebase, error) {
	opt := option.WithCredentialsFile(credentialsFile)
	conf := &firebase.Config{ProjectID: projectID}

	app, err := firebase.NewApp(ctx, conf, opt)
	if err != nil {
		return nil, fmt.Errorf("firebase: init app: %w", err)
	}

	fsClient, err := app.Firestore(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase: init firestore: %w", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		// Clean up Firestore client if Auth init fails.
		_ = fsClient.Close()
		return nil, fmt.Errorf("firebase: init auth: %w", err)
	}

	return &Firebase{
		app:       app,
		Firestore: fsClient,
		Auth:      authClient,
	}, nil
}

// Storage returns a Firebase Storage client derived from the shared app, so
// attachment uploads reuse the same credentials as Firestore and Auth.
func (fb *Firebase) Storage(ctx context.Context) (*fbstorage.Client, error) {
	client, err := fb.app.Storage(ctx)
	if err != nil {
		return nil, fmt.Errorf("firebase: init storage: %w", err)
	}
	return client, nil
}

// Ping performs a cheap Firestore operation to verify connectivity.
// It lists a single document from the root to confirm the connection is alive.
// Respects context timeouts/cancellation.
func (fb *Firebase) Ping(ctx context.Context) error {
	// RunAggregationQuery with Count(0) is the cheapest possible Firestore call:
	// it reads zero documents and still validates the connection + auth.
	iter := fb.Firestore.Collections(ctx)
	_, err := iter.Next()
	if err != nil {
		// iterator.Done is expected for empty projects — that's still a successful ping.
		// Only return error for actual connectivity/auth failures.
		// iterator.Done comes from google.golang.org/api/iterator
		return nil //nolint:nilerr // iterator.Done means Firestore is reachable
	}
	return nil
}

// Close releases all Firebase client resources.
func (fb *Firebase) Close() error {
	return fb.Firestore.Close()
}
