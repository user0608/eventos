/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package sse

import (
	"net/url"
	"sync/atomic"
)

// Stream ...
type Stream struct {
	ID          string
	event       chan *Event
	quit        chan struct{}
	register    chan *Subscriber
	deregister  chan *Subscriber
	subscribers map[string][]*Subscriber
	//subscribers     []*Subscriber
	Eventlog        EventLog
	subscriberCount int32
	// Enables replaying of eventlog to newly added subscribers
	AutoReplay   bool
	isAutoStream bool

	// Specifies the function to run when client subscribe or un-subscribe
	OnSubscribe   func(streamID string, sub *Subscriber)
	OnUnsubscribe func(streamID string, sub *Subscriber)
}

// newStream returns a new stream
func newStream(id string, buffSize int, replay, isAutoStream bool, onSubscribe, onUnsubscribe func(string, *Subscriber)) *Stream {
	return &Stream{
		ID:         id,
		AutoReplay: replay,
		//subscribers:   make([]*Subscriber, 0),
		subscribers:   make(map[string][]*Subscriber),
		isAutoStream:  isAutoStream,
		register:      make(chan *Subscriber),
		deregister:    make(chan *Subscriber),
		event:         make(chan *Event, buffSize),
		quit:          make(chan struct{}),
		Eventlog:      make(EventLog, 0),
		OnSubscribe:   onSubscribe,
		OnUnsubscribe: onUnsubscribe,
	}
}

func (str *Stream) run() {
	go func(str *Stream) {
		for {
			select {
			// Add new subscriber
			case subscriber := <-str.register:
				//str.subscribers = append(str.subscribers, subscriber)
				// Name identifies a user with multiple open connections
				connections, ok := str.subscribers[subscriber.Name]
				if !ok {
					connections = []*Subscriber{subscriber}
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

			// Publish event to subscribers
			case event := <-str.event:
				if str.AutoReplay {
					str.Eventlog.Add(event)
				}
				// for i := range str.subscribers {
				// 	//str.subscribers[i].connection <- event
				// }
				for _, connections := range str.subscribers {
					for index := range connections {
						connections[index].connection <- event
					}
				}

			// Shutdown if the server closes
			case <-str.quit:
				// remove connections
				str.removeAllSubscribers()
				return
			}
		}
	}(str)
}

func (str *Stream) close() {
	str.quit <- struct{}{}
}

func (str *Stream) getSubIndex(sub *Subscriber) (string, int) {
	for name, connection := range str.subscribers {
		for index := range connection {
			if connection[index] == sub {
				return name, index
			}
		}
		// if str.subscribers[i] == sub {
		// 	return i
		// }
	}
	return "", -1
}

// addSubscriber will create a new subscriber on a stream
func (str *Stream) addSubscriber(name string, eventid int, url *url.URL) *Subscriber {
	atomic.AddInt32(&str.subscriberCount, 1)
	sub := &Subscriber{
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

// func (str *Stream) removeSubscriber(i int) {
// 	atomic.AddInt32(&str.subscriberCount, -1)
// 	close(str.subscribers[i].connection)
// 	if str.subscribers[i].removed != nil {
// 		str.subscribers[i].removed <- struct{}{}
// 		close(str.subscribers[i].removed)
// 	}
// 	str.subscribers = append(str.subscribers[:i], str.subscribers[i+1:]...)
// }

func (str *Stream) removeSubscriber(name string, index int) {
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

func (str *Stream) removeAllSubscribers() {
	// for i := 0; i < len(str.subscribers); i++ {
	// 	close(str.subscribers[i].connection)
	// 	if str.subscribers[i].removed != nil {
	// 		str.subscribers[i].removed <- struct{}{}
	// 		close(str.subscribers[i].removed)
	// 	}
	// }
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
	str.subscribers = make(map[string][]*Subscriber)
}

func (str *Stream) getSubscriberCount() int {
	return int(atomic.LoadInt32(&str.subscriberCount))
}
