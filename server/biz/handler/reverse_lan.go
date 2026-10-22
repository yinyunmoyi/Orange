package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"

	"aaa_word/biz/model"
	"aaa_word/biz/reverselan"

	"github.com/cloudwego/hertz/pkg/app"
)

const maxReverseLanRegistrationBytes = 4096

var reverseLanHandlerState struct {
	sync.RWMutex
	enabled  bool
	registry *reverselan.Registry
}

func ConfigureReverseLanRegistration(enabled bool, registry *reverselan.Registry) {
	reverseLanHandlerState.Lock()
	defer reverseLanHandlerState.Unlock()
	reverseLanHandlerState.enabled = enabled
	reverseLanHandlerState.registry = registry
}

func RegisterReverseLanClient(_ context.Context, c *app.RequestContext) {
	reverseLanHandlerState.RLock()
	enabled := reverseLanHandlerState.enabled
	registry := reverseLanHandlerState.registry
	reverseLanHandlerState.RUnlock()
	if !enabled || registry == nil {
		writeReverseLanRegistrationError(c, http.StatusNotFound, "reverse LAN unavailable")
		return
	}

	var request model.ReverseLanRegistrationRequest
	if err := decodeStrictJSON(c.Request.Body(), &request); err != nil {
		writeReverseLanRegistrationError(c, http.StatusBadRequest, "invalid registration")
		return
	}
	ticket, err := registry.Register(reverselan.Registration{
		APIVersion: request.APIVersion,
		ClientID:   request.ClientID,
		ServerID:   request.ServerID,
		Generation: request.Generation,
		Secret:     request.Secret,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, reverselan.ErrRegistrationMismatch) {
			status = http.StatusForbidden
		}
		log.Printf("[reverse-lan] registration rejected client=%s generation=%s reason=%T",
			request.ClientID, request.Generation, err)
		writeReverseLanRegistrationError(c, status, "registration rejected")
		return
	}
	log.Printf("[reverse-lan] registration accepted client=%s generation=%s",
		ticket.ClientID, ticket.Generation)
	c.JSON(http.StatusOK, model.ReverseLanRegistrationResponse{
		Code: 0,
		Data: &model.ReverseLanRegistrationData{
			ExpiresInSeconds: int(reverselan.DefaultTicketTTL.Seconds()),
		},
	})
}

func decodeStrictJSON(body []byte, target any) error {
	if len(body) == 0 || len(body) > maxReverseLanRegistrationBytes {
		return errors.New("invalid body size")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON data")
	}
	return nil
}

func writeReverseLanRegistrationError(c *app.RequestContext, status int, message string) {
	c.JSON(status, model.ReverseLanRegistrationResponse{Code: status, Msg: message})
}
