package room

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrRoomClosed = errors.New("room actor is closed")

type Actor struct {
	inbox chan command
	done  chan struct{}
	once  sync.Once
}

type command struct {
	event                *Event
	eventSubscriptionID  string
	subscribe            chan<- subscription
	subscribePlayerID    string
	unsubscribe          string
	unsubscribePermanent bool
	reply                chan result
}

type subscription struct {
	id       string
	playerID string
	events   chan Envelope
}

type result struct {
	envelope *Envelope
	err      error
}

func NewActor(roomID string) *Actor {
	return newActor(roomID, 0, DefaultPhaseDurations(), nil)
}

func newActor(roomID string, idleTimeout time.Duration, phaseDurations PhaseDurations, onIdle func(*Actor)) *Actor {
	return newActorWithRuleSet(roomID, idleTimeout, phaseDurations, DefaultRuleSet(), onIdle)
}

func newActorWithRuleSet(roomID string, idleTimeout time.Duration, phaseDurations PhaseDurations, rules *RuleSet, onIdle func(*Actor)) *Actor {
	actor := &Actor{
		inbox: make(chan command),
		done:  make(chan struct{}),
	}
	go actor.run(newStateWithRuleSet(roomID, phaseDurations, rules), idleTimeout, onIdle)
	return actor
}

func (a *Actor) Dispatch(ctx context.Context, event Event) (*Envelope, error) {
	return a.dispatch(ctx, "", event)
}

func (a *Actor) DispatchFrom(ctx context.Context, subscriptionID string, event Event) (*Envelope, error) {
	return a.dispatch(ctx, subscriptionID, event)
}

func (a *Actor) dispatch(ctx context.Context, subscriptionID string, event Event) (*Envelope, error) {
	reply := make(chan result, 1)
	cmd := command{
		event:               &event,
		eventSubscriptionID: subscriptionID,
		reply:               reply,
	}

	select {
	case a.inbox <- cmd:
	case <-a.done:
		return nil, ErrRoomClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case result := <-reply:
		return result.envelope, result.err
	case <-a.done:
		return nil, ErrRoomClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Actor) Subscribe(ctx context.Context, playerID string) (<-chan Envelope, func(), error) {
	events, _, disconnect, err := a.SubscribeConnection(ctx, playerID)
	if err != nil {
		return nil, nil, err
	}
	return events, func() { disconnect(false) }, nil
}

func (a *Actor) SubscribeConnection(ctx context.Context, playerID string) (<-chan Envelope, string, func(bool), error) {
	reply := make(chan subscription, 1)

	select {
	case a.inbox <- command{subscribe: reply, subscribePlayerID: playerID}:
	case <-a.done:
		return nil, "", nil, ErrRoomClosed
	case <-ctx.Done():
		return nil, "", nil, ctx.Err()
	}

	var sub subscription
	select {
	case sub = <-reply:
	case <-a.done:
		return nil, "", nil, ErrRoomClosed
	case <-ctx.Done():
		return nil, "", nil, ctx.Err()
	}

	done := func(permanent bool) {
		select {
		case a.inbox <- command{unsubscribe: sub.id, unsubscribePermanent: permanent}:
		case <-a.done:
		}
	}

	return sub.events, sub.id, done, nil
}

func (a *Actor) run(state *State, idleTimeout time.Duration, onIdle func(*Actor)) {
	subscribers := make(map[string]subscription)
	var idleTimer *time.Timer
	var idleC <-chan time.Time
	var phaseTimer *time.Timer
	var phaseC <-chan time.Time

	stopIdleTimer := func() {
		if idleTimer == nil {
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer = nil
		idleC = nil
	}

	resetIdleTimer := func() {
		if idleTimeout <= 0 || len(subscribers) > 0 {
			stopIdleTimer()
			return
		}
		if idleTimer == nil {
			idleTimer = time.NewTimer(idleTimeout)
			idleC = idleTimer.C
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idleTimeout)
	}

	stopPhaseTimer := func() {
		if phaseTimer == nil {
			return
		}
		if !phaseTimer.Stop() {
			select {
			case <-phaseTimer.C:
			default:
			}
		}
		phaseTimer = nil
		phaseC = nil
	}

	resetPhaseTimer := func() {
		if state.phaseDeadline.IsZero() {
			stopPhaseTimer()
			return
		}

		remaining := time.Until(state.phaseDeadline)
		if phaseTimer == nil {
			phaseTimer = time.NewTimer(remaining)
			phaseC = phaseTimer.C
			return
		}
		if !phaseTimer.Stop() {
			select {
			case <-phaseTimer.C:
			default:
			}
		}
		phaseTimer.Reset(remaining)
	}

	defer stopIdleTimer()
	defer stopPhaseTimer()
	resetIdleTimer()
	resetPhaseTimer()

	for {
		select {
		case <-idleC:
			a.close()
			if onIdle != nil {
				onIdle(a)
			}
			return
		case <-phaseC:
			subscriberCount := len(subscribers)
			envelope, err := state.Apply(Event{
				Type: EventPhaseTimeout,
				At:   time.Now().UTC(),
			})
			if err == nil && envelope != nil {
				broadcast(state, subscribers, *envelope)
			}
			resetPhaseTimer()
			if subscriberCount > 0 && len(subscribers) == 0 {
				resetIdleTimer()
			}
		case cmd := <-a.inbox:
			switch {
			case cmd.event != nil:
				event := *cmd.event
				if cmd.eventSubscriptionID != "" {
					sub, ok := subscribers[cmd.eventSubscriptionID]
					if !ok || sub.playerID != event.PlayerID {
						cmd.reply <- result{err: ErrConnectionReplaced}
						break
					}
				}
				envelope, err := state.Apply(event)
				if err == nil && envelope != nil {
					if event.Type == EventKickParticipant {
						disconnectParticipant(subscribers, event.TargetID, ErrKicked)
					}
					broadcast(state, subscribers, *envelope)
				}
				cmd.reply <- result{envelope: envelope, err: err}
			case cmd.subscribe != nil:
				disconnectParticipant(subscribers, cmd.subscribePlayerID, ErrConnectionReplaced)
				sub := subscription{
					id:       randomSubscriptionID(),
					playerID: cmd.subscribePlayerID,
					events:   make(chan Envelope, 16),
				}
				subscribers[sub.id] = sub
				sub.events <- *state.EnvelopeForPlayer(sub.playerID)
				cmd.subscribe <- sub
			case cmd.unsubscribe != "":
				if sub, ok := subscribers[cmd.unsubscribe]; ok {
					delete(subscribers, cmd.unsubscribe)
					close(sub.events)
					eventType := EventDisconnect
					if cmd.unsubscribePermanent {
						eventType = EventLeave
					}
					envelope, err := state.Apply(Event{
						Type:     eventType,
						PlayerID: sub.playerID,
						At:       time.Now().UTC(),
					})
					if err == nil && envelope != nil {
						broadcast(state, subscribers, *envelope)
					}
				}
			}
			resetIdleTimer()
			resetPhaseTimer()
		}
	}
}

func disconnectParticipant(subscribers map[string]subscription, playerID string, reason error) {
	for id, sub := range subscribers {
		if sub.playerID != playerID {
			continue
		}
		for {
			select {
			case <-sub.events:
			default:
				sub.events <- Envelope{Type: "error", Error: reason.Error()}
				delete(subscribers, id)
				close(sub.events)
				goto nextSubscription
			}
		}
	nextSubscription:
	}
}

func (a *Actor) close() {
	a.once.Do(func() {
		close(a.done)
	})
}

func (a *Actor) Closed() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

func broadcast(state *State, subscribers map[string]subscription, envelope Envelope) {
	disconnected := make([]string, 0)
	for id, sub := range subscribers {
		personalized := state.Personalize(envelope, sub.playerID)
		select {
		case sub.events <- personalized:
		default:
			delete(subscribers, id)
			close(sub.events)
			disconnected = append(disconnected, sub.playerID)
		}
	}
	for _, playerID := range disconnected {
		disconnectedEnvelope, err := state.Apply(Event{
			Type:     EventDisconnect,
			PlayerID: playerID,
			At:       time.Now().UTC(),
		})
		if err == nil && disconnectedEnvelope != nil {
			broadcast(state, subscribers, *disconnectedEnvelope)
		}
	}
}
