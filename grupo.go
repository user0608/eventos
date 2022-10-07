/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package sse

import (
	"net/url"
	"sync/atomic"
)

// Grupo ...
type Grupo struct {
	ID              string
	event           chan *Event
	targetEvent     chan *TargetEvent
	quit            chan struct{}
	register        chan *Connection
	deregister      chan *Connection
	subscribers     map[string][]*Connection
	Eventlog        EventLog
	subscriberCount int32
	AutoReplay      bool
	isAutoStream    bool

	OnSubscribe   func(streamID string, sub *Connection)
	OnUnsubscribe func(streamID string, sub *Connection)
}

// newStream returns a new stream
func newGrupo(id string, buffSize int, replay, isAutoStream bool, onSubscribe, onUnsubscribe func(string, *Connection)) *Grupo {
	return &Grupo{
		ID:            id,
		AutoReplay:    replay,
		subscribers:   make(map[string][]*Connection),
		isAutoStream:  isAutoStream,
		register:      make(chan *Connection),
		deregister:    make(chan *Connection),
		event:         make(chan *Event, buffSize),
		targetEvent:   make(chan *TargetEvent, buffSize),
		quit:          make(chan struct{}),
		Eventlog:      make(EventLog, 0),
		OnSubscribe:   onSubscribe,
		OnUnsubscribe: onUnsubscribe,
	}
}

func (str *Grupo) run() {
	go func(str *Grupo) {
		for {
			select {
			// Add new subscriber
			case subscriber := <-str.register:
				connections, ok := str.subscribers[subscriber.Name]
				if !ok {
					connections = []*Connection{subscriber}
				} else {
					connections = append(connections, subscriber)
				}
				str.subscribers[subscriber.Name] = connections

				if str.AutoReplay {
					str.Eventlog.Replay(subscriber)
				}

			// Remove closed subscriber
			case subscriber := <-str.deregister:
				name, index := str.getSubIndex(subscriber)
				if index != -1 {
					str.removeSubscriber(name, index)
				}

				if str.OnUnsubscribe != nil {
					go str.OnUnsubscribe(str.ID, subscriber)
				}

			// Publish event to all grupo subscribers
			case event := <-str.event:
				if str.AutoReplay {
					str.Eventlog.Add(event)
				}
				for _, connections := range str.subscribers {
					for index := range connections {
						connections[index].connection <- event
					}
				}
			case event := <-str.targetEvent:
				connections, ok := str.subscribers[event.ClientName]
				if ok {
					for index := range connections {
						connections[index].connection <- event.Event
					}
				}
			case <-str.quit:
				str.removeAllSubscribers()
				return
			}
		}
	}(str)
}

func (str *Grupo) close() {
	str.quit <- struct{}{}
}

func (str *Grupo) getSubIndex(sub *Connection) (string, int) {
	for name, connection := range str.subscribers {
		for index := range connection {
			if connection[index] == sub {
				return name, index
			}
		}
	}
	return "", -1
}

// addSubscriber will create a new subscriber on a stream
func (str *Grupo) addSubscriber(name string, eventid int, url *url.URL) *Connection {
	atomic.AddInt32(&str.subscriberCount, 1)
	sub := &Connection{
		Name:       name,
		eventid:    eventid,
		quit:       str.deregister,
		connection: make(chan *Event, 64),
		URL:        url,
	}

	if str.isAutoStream {
		sub.removed = make(chan struct{}, 1)
	}

	str.register <- sub

	if str.OnSubscribe != nil {
		go str.OnSubscribe(str.ID, sub)
	}

	return sub
}

func (str *Grupo) removeSubscriber(name string, index int) {
	atomic.AddInt32(&str.subscriberCount, -1)
	connections, ok := str.subscribers[name]
	if !ok {
		return
	}
	close(connections[index].connection)
	if connections[index].removed != nil {
		connections[index].removed <- struct{}{}
		close(connections[index].removed)
	}
	str.subscribers[name] = append(str.subscribers[name][:index], str.subscribers[name][index+1:]...)
}

func (str *Grupo) removeAllSubscribers() {
	for _, connections := range str.subscribers {
		for index := range connections {
			close(connections[index].connection)
			if connections[index].removed != nil {
				connections[index].removed <- struct{}{}
				close(connections[index].removed)
			}
		}
	}
	atomic.StoreInt32(&str.subscriberCount, 0)
	str.subscribers = make(map[string][]*Connection)
}

func (str *Grupo) getSubscriberCount() int {
	return int(atomic.LoadInt32(&str.subscriberCount))
}
