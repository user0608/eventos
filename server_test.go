/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package eventos

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func wait(ch chan *Event, duration time.Duration) ([]byte, error) {
	var err error
	var msg []byte

	select {
	case event := <-ch:
		msg = event.Data
	case <-time.After(duration):
		err = errors.New("timeout")
	}
	return msg, err
}

func waitEvent(ch chan *Event, duration time.Duration) (*Event, error) {
	select {
	case event := <-ch:
		return event, nil
	case <-time.After(duration):
		return nil, errors.New("timeout")
	}
}

func TestServerCreateStream(t *testing.T) {
	s := New()
	defer s.Close()

	s.CreateGrupo("test")

	assert.NotNil(t, s.getGrupo("test"))
}

func TestServerWithCallback(t *testing.T) {
	funcA := func(string, *Connection) {}
	funcB := func(string, *Connection) {}

	s := NewWithCallback(funcA, funcB)
	defer s.Close()
	assert.NotNil(t, s.OnSubscribe)
	assert.NotNil(t, s.OnUnsubscribe)
}

func TestServerCreateExistingStream(t *testing.T) {
	s := New()
	defer s.Close()

	s.CreateGrupo("test")

	numGoRoutines := runtime.NumGoroutine()

	s.CreateGrupo("test")

	assert.NotNil(t, s.getGrupo("test"))
	assert.Equal(t, numGoRoutines, runtime.NumGoroutine())
}

func TestServerRemoveStream(t *testing.T) {
	s := New()
	defer s.Close()

	s.CreateGrupo("test")
	s.RemoveStream("test")

	assert.Nil(t, s.getGrupo("test"))
}

func TestServerRemoveNonExistentStream(t *testing.T) {
	s := New()
	defer s.Close()

	s.RemoveStream("test")

	assert.NotPanics(t, func() { s.RemoveStream("test") })
}

func TestServerExistingStreamPublish(t *testing.T) {
	s := New()
	defer s.Close()

	s.CreateGrupo("test")
	stream := s.getGrupo("test")
	sub := stream.addSubscriber("prueba", 0, nil)

	s.Publish("test", &Event{Data: []byte("test")})

	msg, err := wait(sub.connection, time.Second*1)
	require.Nil(t, err)
	assert.Equal(t, []byte(`test`), msg)
}

func TestServerNonExistentStreamPublish(t *testing.T) {
	s := New()
	defer s.Close()

	s.RemoveStream("test")

	assert.NotPanics(t, func() { s.Publish("test", &Event{Data: []byte("test")}) })
}
