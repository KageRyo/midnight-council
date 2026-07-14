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
	event             *Event
	subscribe         chan<- subscription
	subscribePlayerID string
	unsubscribe       string
	reply             chan result
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

func newActor(roomID string, idleTimeout time.Duration, phaseDurations PhaseDurations, onIdle func()) *Actor {
	actor := &Actor{
		inbox: make(chan command),
		done:  make(chan struct{}),
	}
	go actor.run(newState(roomID, phaseDurations), idleTimeout, onIdle)
	return actor
}

func (a *Actor) Dispatch(ctx context.Context, event Event) (*Envelope, error) {
	reply := make(chan result, 1)
	cmd := command{
		event: &event,
		reply: reply,
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
	reply := make(chan subscription, 1)

	select {
	case a.inbox <- command{subscribe: reply, subscribePlayerID: playerID}:
	case <-a.done:
		return nil, nil, ErrRoomClosed
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	var sub subscription
	select {
	case sub = <-reply:
	case <-a.done:
		return nil, nil, ErrRoomClosed
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	done := func() {
		select {
		case a.inbox <- command{unsubscribe: sub.id}:
		case <-a.done:
		}
	}

	return sub.events, done, nil
}

func (a *Actor) run(state *State, idleTimeout time.Duration, onIdle func()) {
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
				onIdle()
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
				envelope, err := state.Apply(*cmd.event)
				if err == nil && envelope != nil {
					broadcast(state, subscribers, *envelope)
				}
				cmd.reply <- result{envelope: envelope, err: err}
			case cmd.subscribe != nil:
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
				}
			}
			resetIdleTimer()
			resetPhaseTimer()
		}
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
	for id, sub := range subscribers {
		personalized := state.Personalize(envelope, sub.playerID)
		select {
		case sub.events <- personalized:
		default:
			delete(subscribers, id)
			close(sub.events)
		}
	}
}
