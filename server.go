/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package sse

import (
	"encoding/base64"
	"sync"
	"time"
)

// DefaultBufferSize size of the queue that holds the streams messages.
const DefaultBufferSize = 1024

// Server Is our main struct
type Server struct {
	Grupos  map[string]*Grupo
	Headers map[string]string
	// Sets a ttl that prevents old events from being transmitted
	EventTTL time.Duration
	// Specifies the size of the message buffer for each stream
	BufferSize int
	mu         sync.Mutex
	// Encodes all data as base64
	EncodeBase64 bool
	// Splits an events data into multiple data: entries
	SplitData bool
	// Enables creation of a stream when a client connects
	AutoStream bool
	// Enables automatic replay for each new subscriber that connects
	AutoReplay bool

	// Specifies the function to run when client subscribe or un-subscribe
	OnSubscribe   func(streamID string, sub *Connection)
	OnUnsubscribe func(streamID string, sub *Connection)
}

// New will create a server and setup defaults
func New() *Server {
	return &Server{
		BufferSize: DefaultBufferSize,
		AutoStream: false,
		AutoReplay: true,
		Grupos:     make(map[string]*Grupo),
		Headers:    map[string]string{},
	}
}

// NewWithCallback will create a server and setup defaults with callback function
func NewWithCallback(onSubscribe, onUnsubscribe func(streamID string, sub *Connection)) *Server {
	return &Server{
		BufferSize:    DefaultBufferSize,
		AutoStream:    false,
		AutoReplay:    true,
		Grupos:        make(map[string]*Grupo),
		Headers:       map[string]string{},
		OnSubscribe:   onSubscribe,
		OnUnsubscribe: onUnsubscribe,
	}
}

// Close shuts down the server, closes all of the streams and connections
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id := range s.Grupos {
		s.Grupos[id].quit <- struct{}{}
		delete(s.Grupos, id)
	}
}

// CreateStream will create a new stream and register it
func (s *Server) CreateGrupo(id string) *Grupo {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Grupos[id] != nil {
		return s.Grupos[id]
	}

	str := newGrupo(id, s.BufferSize, s.AutoReplay, s.AutoStream, s.OnSubscribe, s.OnUnsubscribe)
	str.run()

	s.Grupos[id] = str

	return str
}

// RemoveStream will remove a stream
func (s *Server) RemoveStream(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Grupos[id] != nil {
		s.Grupos[id].close()
		delete(s.Grupos, id)
	}
}

// StreamExists checks whether a stream by a given id exists
func (s *Server) StreamExists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.Grupos[id] != nil
}

// Publish sends a mesage to every client in a streamID
func (s *Server) Publish(groupName string, event *Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Grupos[groupName] != nil {
		s.Grupos[groupName].event <- s.process(event)
	}
}
func (s *Server) PublicGroupTarget(groupName string, clientname string, event *Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Grupos[groupName] != nil {
		s.Grupos[groupName].targetEvent <- &TargetEvent{ClientName: clientname, Event: event}
	}
}

func (s *Server) getGrupo(id string) *Grupo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Grupos[id]
}

func (s *Server) process(event *Event) *Event {
	if s.EncodeBase64 {
		output := make([]byte, base64.StdEncoding.EncodedLen(len(event.Data)))
		base64.StdEncoding.Encode(output, event.Data)
		event.Data = output
	}
	return event
}
