package services

import (
	"context"
	"sync"

	breezservice "github.com/breez/breez/breez"
	"github.com/breez/breez/config"
	breezlog "github.com/breez/breez/log"
	"github.com/btcsuite/btclog"
	"google.golang.org/grpc"
)

const (
	endpointTimeout = 5
)

// API is the interface for external breez services.
type API interface {
	NewSyncNotifierClient() (breezservice.SyncNotifierClient, context.Context, context.CancelFunc)
	NewFundManager() (breezservice.FundManagerClient, context.Context, context.CancelFunc)
	NewSwapper() (breezservice.SwapperClient, context.Context, context.CancelFunc)
	NewChannelOpenerClient() (breezservice.ChannelOpenerClient, context.Context, context.CancelFunc)
	NewPushTxNotifierClient() (breezservice.PushTxNotifierClient, context.Context, context.CancelFunc)
}

// Client represents the client interface to breez services
type Client struct {
	sync.Mutex
	started    int32
	stopped    int32
	cfg        *config.Config
	log        btclog.Logger
	connection *grpc.ClientConn
}

// NewClient creates a new client struct
func NewClient(cfg *config.Config) (*Client, error) {
	logger, err := breezlog.GetLogger(cfg.WorkingDir, "CLIENT")
	if err != nil {
		return nil, err
	}
	return &Client{
		cfg: cfg,
		log: logger,
	}, nil
}
