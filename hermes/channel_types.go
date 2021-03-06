package main

import (
	pb "hermes/proto"
	"log"

	"hermes/database"
)

type DefaultChannel struct {
	// Maps product_ids to set of subscribed clients
	productClientMap map[string] ClientSet
}

func (dc *DefaultChannel) init(){
	var err error

	// Need to fetch the product IDs from the database first
	// Cache product ids
	if len(acceptedProductIDs) == 0 {
		acceptedProductIDs, err = database.GetProductIDs()
		if err != nil {
			log.Fatalln("Failed setting up channel,", err)
		}
	}

	// Setup the product - client map
	dc.productClientMap = make(map[string] ClientSet)
	for _, id := range acceptedProductIDs {
		dc.productClientMap[id] = make(ClientSet)
	}
}

func (dc *DefaultChannel) subscribeClient(client *Client, productIDs []string){
	for _, id := range productIDs {
		dc.productClientMap[id][client] = true
	}
}

func (dc *DefaultChannel) unsubscribeClient(client *Client, productIDs []string) {
	for _, id := range productIDs {
		delete(dc.productClientMap[id], client)
	}
}

func (dc *DefaultChannel) unsubscribeClientFromAll(client *Client) {
	for id := range dc.productClientMap {
		delete(dc.productClientMap[id], client)
	}
}

func (dc *DefaultChannel) broadcast(productID string, msg interface{}) {
	clients, _ := dc.productClientMap[productID]
	for client := range clients {
		client.message(msg)
	}
}

type HeartbeatChannel struct {
	DefaultChannel
}

type StatusChannel struct {
	DefaultChannel
}

// The ticker channel provides real-time price updates every time a match happens.
type TickerChannel struct {
	DefaultChannel
}

func (tc *TickerChannel) broadcast(productID string, i interface{}) {
	var tickerMsg TickerMessage

	switch msg := i.(type) {
	case pb.TradeMessage:
		tickerMsg = newTickerMessage(productID, msg)
	default:
		log.Fatalf("TickerChannel - Received bad message from server: %+v", msg)
	}

	clients, _ := tc.productClientMap[productID]
	for client := range clients {
		client.message(tickerMsg)
	}
}

type Level2Channel struct {
	DefaultChannel
}

type UserChannel struct {
	DefaultChannel
}

type MatchesChannel struct {
	DefaultChannel
}

type FullChannel struct {
	DefaultChannel
}
