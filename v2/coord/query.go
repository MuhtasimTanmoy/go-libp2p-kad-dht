package coord

import (
	"context"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p-kad-dht/v2/kadt"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/plprobelab/go-kademlia/query"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

type PooledQueryBehaviour struct {
	pool    *query.Pool[KadKey, ma.Multiaddr]
	waiters map[query.QueryID]NotifyCloser[BehaviourEvent]

	pendingMu sync.Mutex
	pending   []BehaviourEvent
	ready     chan struct{}

	logger *slog.Logger
	tracer trace.Tracer
}

func NewPooledQueryBehaviour(pool *query.Pool[KadKey, ma.Multiaddr], logger *slog.Logger, tracer trace.Tracer) *PooledQueryBehaviour {
	h := &PooledQueryBehaviour{
		pool:    pool,
		waiters: make(map[query.QueryID]NotifyCloser[BehaviourEvent]),
		ready:   make(chan struct{}, 1),
		logger:  logger,
		tracer:  tracer,
	}
	return h
}

func (p *PooledQueryBehaviour) Notify(ctx context.Context, ev BehaviourEvent) {
	ctx, span := p.tracer.Start(ctx, "PooledQueryBehaviour.Notify")
	defer span.End()

	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	var cmd query.PoolEvent
	switch ev := ev.(type) {
	case *EventStartQuery:
		cmd = &query.EventPoolAddQuery[KadKey, ma.Multiaddr]{
			QueryID:           ev.QueryID,
			Target:            ev.Target,
			ProtocolID:        ev.ProtocolID,
			Message:           ev.Message,
			KnownClosestNodes: SliceOfPeerIDToSliceOfNodeID(ev.KnownClosestNodes),
		}
		if ev.Notify != nil {
			p.waiters[ev.QueryID] = ev.Notify
		}

	case *EventStopQuery:
		cmd = &query.EventPoolStopQuery{
			QueryID: ev.QueryID,
		}

	case *EventGetCloserNodesSuccess:
		for _, info := range ev.CloserNodes {
			// TODO: do this after advancing pool
			p.pending = append(p.pending, &EventAddAddrInfo{
				NodeInfo: info,
			})
		}
		waiter, ok := p.waiters[ev.QueryID]
		if ok {
			waiter.Notify(ctx, &EventQueryProgressed{
				NodeID:   ev.To.ID,
				QueryID:  ev.QueryID,
				Response: CloserNodesResponse(ev.Target, ev.CloserNodes),
				// Stats:    stats,
			})
		}
		cmd = &query.EventPoolMessageResponse[KadKey, ma.Multiaddr]{
			NodeID:   kadt.PeerID(ev.To.ID),
			QueryID:  ev.QueryID,
			Response: CloserNodesResponse(ev.Target, ev.CloserNodes),
		}
	case *EventGetCloserNodesFailure:
		cmd = &query.EventPoolMessageFailure[KadKey]{
			NodeID:  kadt.PeerID(ev.To.ID),
			QueryID: ev.QueryID,
			Error:   ev.Err,
		}
	default:
		panic(fmt.Sprintf("unexpected dht event: %T", ev))
	}

	// attempt to advance the query pool
	ev, ok := p.advancePool(ctx, cmd)
	if ok {
		p.pending = append(p.pending, ev)
	}
	if len(p.pending) > 0 {
		select {
		case p.ready <- struct{}{}:
		default:
		}
	}
}

func (p *PooledQueryBehaviour) Ready() <-chan struct{} {
	return p.ready
}

func (p *PooledQueryBehaviour) Perform(ctx context.Context) (BehaviourEvent, bool) {
	ctx, span := p.tracer.Start(ctx, "PooledQueryBehaviour.Perform")
	defer span.End()

	// No inbound work can be done until Perform is complete
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	for {
		// drain queued events first.
		if len(p.pending) > 0 {
			var ev BehaviourEvent
			ev, p.pending = p.pending[0], p.pending[1:]

			if len(p.pending) > 0 {
				select {
				case p.ready <- struct{}{}:
				default:
				}
			}
			return ev, true
		}

		// attempt to advance the query pool
		ev, ok := p.advancePool(ctx, &query.EventPoolPoll{})
		if ok {
			return ev, true
		}

		if len(p.pending) == 0 {
			return nil, false
		}
	}
}

func (p *PooledQueryBehaviour) advancePool(ctx context.Context, ev query.PoolEvent) (BehaviourEvent, bool) {
	ctx, span := p.tracer.Start(ctx, "PooledQueryBehaviour.advancePool")
	defer span.End()

	pstate := p.pool.Advance(ctx, ev)
	switch st := pstate.(type) {
	case *query.StatePoolQueryMessage[KadKey, ma.Multiaddr]:
		return &EventOutboundGetCloserNodes{
			QueryID: st.QueryID,
			To:      NodeIDToAddrInfo(st.NodeID),
			Target:  st.Message.Target(),
			Notify:  p,
		}, true
	case *query.StatePoolWaitingAtCapacity:
		// nothing to do except wait for message response or timeout
	case *query.StatePoolWaitingWithCapacity:
		// nothing to do except wait for message response or timeout
	case *query.StatePoolQueryFinished:
		waiter, ok := p.waiters[st.QueryID]
		if ok {
			waiter.Notify(ctx, &EventQueryFinished{
				QueryID: st.QueryID,
				Stats:   st.Stats,
			})
			waiter.Close()
		}
	case *query.StatePoolQueryTimeout:
		// TODO
	case *query.StatePoolIdle:
		// nothing to do
	default:
		panic(fmt.Sprintf("unexpected pool state: %T", st))
	}

	return nil, false
}