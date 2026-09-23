package service

import (
	"context"

	"github.com/kandev/kandev/internal/events/bus"
)

// skipExternalConversation drops events for tasks whose runner is owned by a
// runtime outside Office, so Office never queues or relays work for them.
func (s *Service) skipExternalConversation(handler bus.EventHandler) bus.EventHandler {
	return func(ctx context.Context, event *bus.Event) error {
		external, err := s.isExternalConversation(ctx, event)
		if err != nil {
			return err
		}
		if external {
			return nil
		}
		return handler(ctx, event)
	}
}

func (s *Service) isExternalConversation(ctx context.Context, event *bus.Event) (bool, error) {
	data, err := decodeEventData[TaskUpdatedData](event)
	if err != nil || data.TaskID == "" {
		return false, nil
	}
	return s.repo.IsExternalAgentTask(ctx, data.TaskID)
}
