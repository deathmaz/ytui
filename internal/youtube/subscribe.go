package youtube

import (
	"context"
	"errors"
	"fmt"
)

const (
	endpointSubscribe   = "SUBSCRIPTION/SUBSCRIBE"
	endpointUnsubscribe = "SUBSCRIPTION/UNSUBSCRIBE"
)

// ErrSubscriptionNotConfirmed is returned when a Subscribe/Unsubscribe call
// succeeds at the HTTP layer but the response does not contain an
// updateSubscribeButtonAction matching the requested channel and direction.
// YouTube returns 200 for no-op toggles, so this is the only reliable signal.
var ErrSubscriptionNotConfirmed = errors.New("subscription change not confirmed")

// Subscribe subscribes the signed-in user to channelID.
func (c *InnerTubeClient) Subscribe(ctx context.Context, channelID string) error {
	return c.mutateSubscription(ctx, endpointSubscribe, channelID)
}

// Unsubscribe removes the signed-in user's subscription from channelID.
func (c *InnerTubeClient) Unsubscribe(ctx context.Context, channelID string) error {
	return c.mutateSubscription(ctx, endpointUnsubscribe, channelID)
}

func (c *InnerTubeClient) mutateSubscription(ctx context.Context, endpoint, channelID string) error {
	if !c.authenticated {
		return fmt.Errorf("authentication required for subscription changes")
	}
	if channelID == "" {
		return fmt.Errorf("empty channel ID")
	}
	raw, err := c.callRaw(ctx, endpoint, map[string]interface{}{
		"channelIds": []string{channelID},
	})
	if err != nil {
		return fmt.Errorf("innertube %s: %w", endpoint, err)
	}
	data, err := toGJSON(raw)
	if err != nil {
		return err
	}
	want := endpoint == endpointSubscribe
	query := fmt.Sprintf(`actions.#(updateSubscribeButtonAction.channelId==%q).updateSubscribeButtonAction.subscribed`, channelID)
	result := data.Get(query)
	if !result.Exists() || result.Bool() != want {
		return fmt.Errorf("%w for %s", ErrSubscriptionNotConfirmed, channelID)
	}
	return nil
}
