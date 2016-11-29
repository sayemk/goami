package ami

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

var (
	// ErrInvalidAction occurs when the action type is invalid.
	ErrInvalidAction = errors.New("invalid Action")
)

// GetUUID returns a new UUID based on /dev/urandom (unix).
func GetUUID() (string, error) {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		return "", fmt.Errorf("open /dev/urandom error:[%v]", err)
	}
	defer f.Close()
	b := make([]byte, 16)

	_, err = f.Read(b)
	if err != nil {
		return "", err
	}
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	return uuid, nil
}

func send(client Client, action, id string, v interface{}) (Response, error) {
	if action == "" {
		return nil, ErrInvalidAction
	}
	b, err := marshal(&struct {
		Action string `ami:"Action"`
		ID     string `ami:"ActionID, omitempty"`
		V      interface{}
	}{Action: action, ID: id, V: v})
	if err != nil {
		return nil, err
	}
	if err := client.Send(string(b)); err != nil {
		return nil, err
	}
	input, err := client.Recv()
	log.Println("input: ", spew.Sdump(input))
	if err != nil {
		return nil, err
	}
	return parseResponse(input)
}

func parseResponse(input string) (Response, error) {
	resp := make(Response)
	lines := strings.Split(input, "\r\n")
	for _, line := range lines {
		keys := strings.SplitAfterN(line, ":", 2)
		if len(keys) == 2 {
			key := strings.TrimSpace(strings.Trim(keys[0], ":"))
			value := strings.TrimSpace(keys[1])
			resp[key] = append(resp[key], value)
		} else if strings.Contains(line, "\r\n\r\n") || len(line) == 0 {
			return resp, nil
		}
	}
	return resp, nil
}

const (
	getResponseState int = iota
	getListState
)

func requestList(client Client, action, id, event, complete string) ([]Response, error) {
	if action == "" {
		return nil, ErrInvalidAction
	}
	b, err := marshal(&struct {
		Action string `ami:"Action"`
		ID     string `ami:"ActionID, omitempty"`
	}{Action: action, ID: id})
	if err != nil {
		return nil, err
	}
	if err := client.Send(string(b)); err != nil {
		return nil, err
	}
	input, err := client.Recv()
	if err != nil {
		return nil, err
	}
	commands := strings.Split(input, "\r\n\r\n")
	return parseEvent(event, complete, commands)
}

func parseEvent(event, complete string, input []string) ([]Response, error) {
	var list []Response
	verify := false

	for _, in := range input {
		rsp, err := parseResponse(in)
		if err != nil {
			return nil, err
		}
		switch verify {
		case false:
			if success := rsp.Get("Response"); success != "Success" {
				return nil, fmt.Errorf("failed on event %s:%v\n", event, rsp.Get("Message"))
			}
			verify = true
		case true:
			evt := rsp.Get("Event")
			if evt == complete {
				break
			}
			if evt == event {
				list = append(list, rsp)
			}
		}
	}
	return list, nil
}
