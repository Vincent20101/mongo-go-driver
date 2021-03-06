// Copyright (C) MongoDB, Inc. 2017-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package auth

import (
	"context"

	"github.com/Vincent20101/mongo-go-driver/bson"
	"github.com/Vincent20101/mongo-go-driver/core/command"
	"github.com/Vincent20101/mongo-go-driver/core/description"
	"github.com/Vincent20101/mongo-go-driver/core/wiremessage"
)

// SaslClient is the client piece of a sasl conversation.
type SaslClient interface {
	Start() (string, []byte, error)
	Next(challenge []byte) ([]byte, error)
	Completed() bool
}

// SaslClientCloser is a SaslClient that has resources to clean up.
type SaslClientCloser interface {
	SaslClient
	Close()
}

// ConductSaslConversation handles running a sasl conversation with MongoDB.
func ConductSaslConversation(ctx context.Context, desc description.Server, rw wiremessage.ReadWriter, db string, client SaslClient) error {
	// Arbiters cannot be authenticated
	if desc.Kind == description.RSArbiter {
		return nil
	}

	if db == "" {
		db = defaultAuthDB
	}

	if closer, ok := client.(SaslClientCloser); ok {
		defer closer.Close()
	}

	mech, payload, err := client.Start()
	if err != nil {
		return newError(err, mech)
	}

	saslStartCmd := command.Read{
		DB: db,
		Command: bson.NewDocument(
			bson.EC.Int32("saslStart", 1),
			bson.EC.String("mechanism", mech),
			bson.EC.Binary("payload", payload),
		),
	}

	type saslResponse struct {
		ConversationID int    `bson:"conversationId"`
		Code           int    `bson:"code"`
		Done           bool   `bson:"done"`
		Payload        []byte `bson:"payload"`
	}

	var saslResp saslResponse

	ssdesc := description.SelectedServer{Server: desc}
	rdr, err := saslStartCmd.RoundTrip(ctx, ssdesc, rw)
	if err != nil {
		return newError(err, mech)
	}

	err = bson.Unmarshal(rdr, &saslResp)
	if err != nil {
		return newAuthError("unmarshall error", err)
	}

	cid := saslResp.ConversationID

	for {
		if saslResp.Code != 0 {
			return newError(err, mech)
		}

		if saslResp.Done && client.Completed() {
			return nil
		}

		payload, err = client.Next(saslResp.Payload)
		if err != nil {
			return newError(err, mech)
		}

		if saslResp.Done && client.Completed() {
			return nil
		}

		saslContinueCmd := command.Read{
			DB: db,
			Command: bson.NewDocument(
				bson.EC.Int32("saslContinue", 1),
				bson.EC.Int32("conversationId", int32(cid)),
				bson.EC.Binary("payload", payload),
			),
		}

		rdr, err = saslContinueCmd.RoundTrip(ctx, ssdesc, rw)
		if err != nil {
			return newError(err, mech)
		}

		err = bson.Unmarshal(rdr, &saslResp)
		if err != nil {
			return newAuthError("unmarshal error", err)
		}
	}
}
