package registry

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	log "github.com/sirupsen/logrus"

	"github.com/forta-protocol/forta-core-go/contracts/contract_agent_registry"
	"github.com/forta-protocol/forta-core-go/contracts/contract_dispatch"
	"github.com/forta-protocol/forta-core-go/contracts/contract_scanner_registry"
	"github.com/forta-protocol/forta-core-go/domain"
	"github.com/forta-protocol/forta-core-go/domain/registry"
	"github.com/forta-protocol/forta-core-go/ens"
	"github.com/forta-protocol/forta-core-go/ethereum"
	"github.com/forta-protocol/forta-core-go/feeds"
)

type listener struct {
	ctx  context.Context
	cfg  ListenerConfig
	logs feeds.LogFeed

	scannerAddr  string
	agentAddr    string
	dispatchAddr string

	scannerFilterer  *contract_scanner_registry.ScannerRegistryFilterer
	agentsFilterer   *contract_agent_registry.AgentRegistryFilterer
	dispatchFilterer *contract_dispatch.DispatchFilterer
}

type Handlers struct {
	AfterBlockHandler func(blk *domain.Block) error

	SaveAgentHandler     func(logger *log.Entry, agt *registry.AgentSaveMessage) error
	AgentActionHandler   func(logger *log.Entry, agt *registry.AgentMessage) error
	DispatchHandler      func(logger *log.Entry, agt *registry.DispatchMessage) error
	SaveScannerHandler   func(logger *log.Entry, agt *registry.ScannerSaveMessage) error
	ScannerActionHandler func(logger *log.Entry, agt *registry.ScannerMessage) error
}

type ListenerConfig struct {
	Name        string
	JsonRpcURL  string
	ENSAddress  string
	StartBlock  *big.Int
	BlockOffset int
	Handlers    Handlers
}

type Listener interface {
	Listen() error
	ProcessLastBlocks(blocksAgo int64) error
}

func (l *listener) isDispatcher(address common.Address) bool {
	return equalsAddress(address, l.dispatchAddr)
}

func (l *listener) isScannerRegistry(address common.Address) bool {
	return equalsAddress(address, l.scannerAddr)
}

func (l *listener) isAgentRegistry(address common.Address) bool {
	return equalsAddress(address, l.agentAddr)
}

func (l *listener) handleScannerRegistryEvent(le types.Log, logger *log.Entry) error {
	if isEvent(le, contract_scanner_registry.ScannerUpdatedTopic) {
		su, err := l.scannerFilterer.ParseScannerUpdated(le)
		if err != nil {
			return err
		}
		if l.cfg.Handlers.SaveScannerHandler != nil {
			return l.cfg.Handlers.SaveScannerHandler(logger, registry.NewScannerSaveMessage(su))
		}
	} else if isEvent(le, contract_scanner_registry.ScannerEnabledTopic) {
		se, err := l.scannerFilterer.ParseScannerEnabled(le)
		if err != nil {
			return err
		}
		if l.cfg.Handlers.ScannerActionHandler != nil {
			return l.cfg.Handlers.ScannerActionHandler(logger, registry.NewScannerMessage(se))
		}
	}
	return nil
}

func (l *listener) handleAgentRegistryEvent(le types.Log, logger *log.Entry) error {
	if isEvent(le, contract_agent_registry.AgentUpdatedTopic) {
		au, err := l.agentsFilterer.ParseAgentUpdated(le)
		if err != nil {
			return err
		}
		if l.cfg.Handlers.SaveAgentHandler != nil {
			return l.cfg.Handlers.SaveAgentHandler(logger, registry.NewAgentSaveMessage(au))
		}
	} else if isEvent(le, contract_agent_registry.AgentEnabledTopic) {
		ae, err := l.agentsFilterer.ParseAgentEnabled(le)
		if err != nil {
			return err
		}
		if l.cfg.Handlers.AgentActionHandler != nil {
			return l.cfg.Handlers.AgentActionHandler(logger, registry.NewAgentMessage(ae))
		}
	}
	return nil
}

func (l *listener) handleDispatcherEvent(le types.Log, logger *log.Entry) error {
	if isEvent(le, contract_dispatch.LinkTopic) {
		link, err := l.dispatchFilterer.ParseLink(le)
		if err != nil {
			return err
		}
		if l.cfg.Handlers.DispatchHandler != nil {
			return l.cfg.Handlers.DispatchHandler(logger, registry.NewDispatchMessage(link))
		}
	}
	return nil
}

func (l *listener) handleLog(blk *domain.Block, le types.Log) error {
	if l.ctx.Err() != nil {
		return l.ctx.Err()
	}
	logger := getLoggerForLog(le)
	if l.isAgentRegistry(le.Address) {
		return l.handleAgentRegistryEvent(le, logger)
	}
	if l.isDispatcher(le.Address) {
		return l.handleDispatcherEvent(le, logger)
	}
	if l.isScannerRegistry(le.Address) {
		return l.handleScannerRegistryEvent(le, logger)
	}
	return nil
}

func (l *listener) handleAfterBlock(blk *domain.Block) error {
	if l.ctx.Err() != nil {
		return l.ctx.Err()
	}
	if l.cfg.Handlers.AfterBlockHandler != nil {
		return l.cfg.Handlers.AfterBlockHandler(blk)
	}
	return nil
}

// ProcessLogs fetches the logs in a single pass and calls handlers for them
func (l *listener) ProcessLastBlocks(blocksAgo int64) error {
	logs, err := l.logs.GetLogsForLastBlocks(blocksAgo)
	if err != nil {
		return err
	}
	for _, lg := range logs {
		if err := l.handleLog(nil, lg); err != nil {
			return err
		}
	}
	return nil
}

func (l *listener) Listen() error {
	return l.logs.ForEachLog(l.handleLog, l.handleAfterBlock)
}

func NewListener(ctx context.Context, cfg ListenerConfig) (*listener, error) {
	client, err := ethereum.NewStreamEthClient(ctx, cfg.Name, cfg.JsonRpcURL)
	if err != nil {
		return nil, err
	}
	ensStore, err := ens.DialENSStoreAt(cfg.JsonRpcURL, cfg.ENSAddress)
	if err != nil {
		return nil, err
	}

	regContracts, err := ensStore.ResolveRegistryContracts()

	sf, err := contract_scanner_registry.NewScannerRegistryFilterer(regContracts.ScannerRegistry, nil)
	if err != nil {
		return nil, err
	}

	af, err := contract_agent_registry.NewAgentRegistryFilterer(regContracts.AgentRegistry, nil)
	if err != nil {
		return nil, err
	}

	df, err := contract_dispatch.NewDispatchFilterer(regContracts.Dispatch, nil)
	if err != nil {
		return nil, err
	}

	logFeed, err := feeds.NewLogFeed(ctx, client, feeds.LogFeedConfig{
		Addresses:  []string{regContracts.AgentRegistry.Hex(), regContracts.ScannerRegistry.Hex(), regContracts.Dispatch.Hex()},
		StartBlock: cfg.StartBlock,
		Offset:     cfg.BlockOffset,
	})

	if err != nil {
		return nil, err
	}

	return &listener{
		ctx:              ctx,
		cfg:              cfg,
		logs:             logFeed,
		scannerAddr:      regContracts.ScannerRegistry.Hex(),
		agentAddr:        regContracts.AgentRegistry.Hex(),
		dispatchAddr:     regContracts.Dispatch.Hex(),
		scannerFilterer:  sf,
		agentsFilterer:   af,
		dispatchFilterer: df,
	}, nil
}
