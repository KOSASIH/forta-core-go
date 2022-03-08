package feeds

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/forta-protocol/forta-core-go/clients/health"
	"github.com/forta-protocol/forta-core-go/domain"
	mocks "github.com/forta-protocol/forta-core-go/ethereum/mocks"
	"github.com/forta-protocol/forta-core-go/utils"
)

var testErr = errors.New("test")
var startHash = "0x4fc0862e76691f5312964883954d5c2db35e2b8f7a4f191775a4f50c69804a8d"

var endOfBlocks = errors.New("end of blocks")

// mockBlockFeed is a mock block feed for tests
type mockBlockFeed struct {
	blocks []*domain.BlockEvent
}

// ForEachBlock is a test method that iterates over mocked blocks
func (bf *mockBlockFeed) Subscribe(handler func(evt *domain.BlockEvent) error) <-chan error {
	errCh := make(chan error, 1)
	for _, b := range bf.blocks {
		if err := handler(b); err != nil {
			errCh <- err
			return errCh
		}
	}
	errCh <- endOfBlocks
	return errCh
}

// Start implements the BlockFeed interface.
func (bf *mockBlockFeed) Start() {}

// IsStarted implements the BlockFeed interface.
func (bf *mockBlockFeed) IsStarted() bool {
	return true
}

// StartRange implements the BlockFeed interface.
func (bf *mockBlockFeed) StartRange(start int64, end int64, rate int64) {}

// Name implements the BlockFeed interface.
func (bf *mockBlockFeed) Name() string {
	return "mock-block-feed"
}

// Health implements the BlockFeed interface.
func (bf *mockBlockFeed) Health() health.Reports {
	return nil
}

// NewMockBlockFeed returns a new mockBlockFeed for tests
func NewMockBlockFeed(blocks []*domain.BlockEvent) *mockBlockFeed {
	return &mockBlockFeed{blocks}
}

func getTestBlockFeed(t *testing.T) (*blockFeed, *mocks.MockClient, *mocks.MockClient, context.Context, context.CancelFunc) {
	ctrl := gomock.NewController(t)
	client := mocks.NewMockClient(ctrl)
	traceClient := mocks.NewMockClient(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	cache := utils.NewCache(10000)
	maxBlockAge := time.Hour
	return &blockFeed{
		start:       big.NewInt(1),
		ctx:         ctx,
		client:      client,
		traceClient: traceClient,
		cache:       cache,
		tracing:     true,
		maxBlockAge: &maxBlockAge,
	}, client, traceClient, ctx, cancel
}

func blockWithParent(hash string, num int) *domain.Block {
	ts := utils.BigIntToHex(big.NewInt(time.Now().Unix()))
	return &domain.Block{
		Hash:       fmt.Sprintf("0x%s%d", hash, num),
		ParentHash: hash,
		Number:     utils.BigIntToHex(big.NewInt(int64(num))),
		Timestamp:  ts,
	}
}

func blockEvent(blk *domain.Block) *domain.BlockEvent {
	return &domain.BlockEvent{
		EventType: domain.EventTypeBlock,
		Block:     blk,
	}
}

func assertEvts(t *testing.T, actual []*domain.BlockEvent, expected ...*domain.BlockEvent) {
	assert.Equal(t, len(actual), len(expected), "expect same length")
	for i, exp := range expected {
		assert.Equal(t, exp, actual[i])
	}
}

func hexToBigInt(hex string) *big.Int {
	bi, _ := utils.HexToBigInt(hex)
	return bi
}

func TestBlockFeed_ForEachBlock(t *testing.T) {
	bf, client, traceClient, ctx, _ := getTestBlockFeed(t)

	block1 := blockWithParent(startHash, 1)
	block2 := blockWithParent(block1.Hash, 2)
	block3 := blockWithParent(block2.Hash, 3)

	//TODO: actually test that the trace part matters (this returns nil for now)
	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(1), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(1)).Return(block1, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block1.Number)).Return(nil, nil).Times(1)

	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(2), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(2)).Return(block2, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block2.Number)).Return(nil, nil).Times(1)

	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(3), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(3)).Return(block3, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block3.Number)).Return(nil, nil).Times(1)

	count := 0
	var evts []*domain.BlockEvent
	bf.Subscribe(func(evt *domain.BlockEvent) error {
		count++
		evts = append(evts, evt)
		if count == 3 {
			return testErr
		}
		return nil
	})
	res := bf.forEachBlock()
	assert.Error(t, testErr, res)
	assert.Equal(t, 3, len(evts))
	assertEvts(t, evts, blockEvent(block1), blockEvent(block2), blockEvent(block3))
}

func TestBlockFeed_ForEachBlockWithOldBlock(t *testing.T) {
	bf, client, traceClient, ctx, _ := getTestBlockFeed(t)

	block1 := blockWithParent(startHash, 1)
	block2 := blockWithParent(block1.Hash, 2)
	block2.Timestamp = utils.BigIntToHex(big.NewInt(time.Now().Add(-2 * time.Hour).Unix()))

	block3 := blockWithParent(block2.Hash, 3)

	//TODO: actually test that the trace part matters (this returns nil for now)
	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(1), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(1)).Return(block1, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block1.Number)).Return(nil, nil).Times(1)

	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(2), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(2)).Return(block2, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block2.Number)).Return(nil, nil).Times(1)

	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(3), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(3)).Return(block3, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block3.Number)).Return(nil, nil).Times(1)

	count := 0
	var evts []*domain.BlockEvent
	bf.Subscribe(func(evt *domain.BlockEvent) error {
		count++
		evts = append(evts, evt)
		if count == 2 {
			return testErr
		}
		return nil
	})
	res := bf.forEachBlock()
	assert.Error(t, testErr, res)
	assert.Equal(t, 2, len(evts))

	// should skip block 2 and continue to block 3
	assertEvts(t, evts, blockEvent(block1), blockEvent(block3))
}

func TestBlockFeed_ForEachBlock_Cancelled(t *testing.T) {
	bf, client, traceClient, ctx, cancel := getTestBlockFeed(t)

	hash1 := "0x4fc0862e76691f5312964883954d5c2db35e2b8f7a4f191775a4f50c69804a8d"
	block1 := blockWithParent(hash1, 1)

	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(1), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(1)).Return(block1, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block1.Number)).Return(nil, nil).Times(1)

	count := 0
	var evts []*domain.BlockEvent
	bf.Subscribe(func(evt *domain.BlockEvent) error {
		count++
		evts = append(evts, evt)
		cancel()
		return nil
	})
	res := bf.forEachBlock()
	assert.Error(t, context.Canceled, res)
	assert.Equal(t, 1, len(evts))
	assertEvts(t, evts, blockEvent(block1))
}

func TestBlockFeed_ForEachBlock_WithOffset(t *testing.T) {
	bf, client, traceClient, ctx, _ := getTestBlockFeed(t)
	bf.offset = 1            // use a simple offset of 1
	bf.start = big.NewInt(2) // make it 2 so we don't request block 0

	block1 := blockWithParent(startHash, 1)
	block2 := blockWithParent(block1.Hash, 2)
	block3 := blockWithParent(block2.Hash, 3)

	// current block number is 2, latest block number is 3: get block 1
	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(3), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(1)).Return(block1, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block1.Number)).Return(nil, nil).Times(1)

	// current block number is 3, latest block number is 3: get block 2
	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(3), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(2)).Return(block2, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block2.Number)).Return(nil, nil).Times(1)

	// current block number is 4, latest block number is 3: skip
	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(3), nil).Times(1)

	// current block number is 4 again, latest block number is 3: skip
	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(3), nil).Times(1)

	// current block number is 4, latest block number is 4: get block 3
	client.EXPECT().BlockNumber(ctx).Return(big.NewInt(4), nil).Times(1)
	client.EXPECT().BlockByNumber(ctx, big.NewInt(3)).Return(block3, nil).Times(1)
	traceClient.EXPECT().TraceBlock(ctx, hexToBigInt(block3.Number)).Return(nil, nil).Times(1)

	count := 0
	var evts []*domain.BlockEvent
	bf.Subscribe(func(evt *domain.BlockEvent) error {
		count++
		evts = append(evts, evt)
		if count == 3 {
			return testErr
		}
		return nil
	})
	res := bf.forEachBlock()
	assert.Error(t, testErr, res)
	assert.Equal(t, 3, len(evts))
	assertEvts(t, evts, blockEvent(block1), blockEvent(block2), blockEvent(block3))
}
