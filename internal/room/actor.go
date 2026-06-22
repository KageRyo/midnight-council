package room

import (
	"context"
)

type Actor struct {
	inbox chan command
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
	actor := &Actor{
		inbox: make(chan command),
	}
	go actor.run(NewState(roomID))
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
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case result := <-reply:
		return result.envelope, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Actor) Subscribe(ctx context.Context, playerID string) (<-chan Envelope, func(), error) {
	reply := make(chan subscription, 1)

	select {
	case a.inbox <- command{subscribe: reply, subscribePlayerID: playerID}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	var sub subscription
	select {
	case sub = <-reply:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	done := func() {
		a.inbox <- command{unsubscribe: sub.id}
	}

	return sub.events, done, nil
}

func (a *Actor) run(state *State) {
	subscribers := make(map[string]subscription)

	for cmd := range a.inbox {
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
