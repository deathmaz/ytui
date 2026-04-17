package youtube

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// GetChannel fetches the channel header + metadata for a channel ID or @handle.
// Handles are auto-resolved to UC IDs via NAVIGATION/RESOLVE_URL because
// plain BROWSE rejects handles with a 400.
func (c *InnerTubeClient) GetChannel(ctx context.Context, channelID string) (*ChannelDetail, error) {
	id := channelID
	if strings.HasPrefix(id, "@") {
		resolved, err := c.resolveHandle(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", id, err)
		}
		id = resolved
	}

	raw, err := c.callRaw(ctx, "BROWSE", map[string]interface{}{"browseId": id})
	if err != nil {
		return nil, fmt.Errorf("innertube channel browse: %w", err)
	}
	data, err := toGJSON(raw)
	if err != nil {
		return nil, err
	}
	return parseChannelDetail(data), nil
}

func (c *InnerTubeClient) resolveHandle(ctx context.Context, handle string) (string, error) {
	resp, err := c.callRaw(ctx, "NAVIGATION/RESOLVE_URL", map[string]interface{}{
		"url": "https://www.youtube.com/" + handle,
	})
	if err != nil {
		return "", err
	}
	data, err := toGJSON(resp)
	if err != nil {
		return "", err
	}
	id := data.Get("endpoint.browseEndpoint.browseId").String()
	if id == "" {
		return "", fmt.Errorf("no browseId in resolve response")
	}
	return id, nil
}

// parseChannelDetail extracts header + metadata from a channel Browse response.
func parseChannelDetail(data gjson.Result) *ChannelDetail {
	d := &ChannelDetail{}

	cmr := data.Get("metadata.channelMetadataRenderer")
	d.ID = cmr.Get("externalId").String()
	d.Name = cmr.Get("title").String()
	d.Description = cmr.Get("description").String()
	if vanity := cmr.Get("vanityChannelUrl").String(); vanity != "" {
		if p := ParseYouTubeURL(vanity); p.Kind == URLChannel && strings.HasPrefix(p.ID, "@") {
			d.Handle = p.ID
		}
	}
	d.Thumbnails = parseThumbnails(cmr.Get("avatar.thumbnails"))
	if d.ID != "" {
		d.URL = ChannelURL(d.ID)
	}

	// New-format pageHeaderRenderer; older channelHeaderRenderer not handled.
	phvm := data.Get("header.pageHeaderRenderer.content.pageHeaderViewModel")

	// metadataRows: row 0 holds the @handle; row 1 holds [subscriberCount,
	// videoCount]. Text is locale-opaque (e.g. "4.38M subscribers") so we
	// index positionally rather than string-match.
	phvm.Get("metadata.contentMetadataViewModel.metadataRows").ForEach(func(rowIdx, row gjson.Result) bool {
		parts := row.Get("metadataParts")
		switch rowIdx.Int() {
		case 0:
			if d.Handle == "" {
				parts.ForEach(func(_, part gjson.Result) bool {
					txt := part.Get("text.content").String()
					if strings.HasPrefix(txt, "@") {
						d.Handle = txt
						return false
					}
					return true
				})
			}
		case 1:
			d.SubscriberCount = parts.Get("0.text.content").String()
			d.VideoCount = parts.Get("1.text.content").String()
		}
		return true
	})

	if d.Description == "" {
		d.Description = phvm.Get("description.descriptionPreviewViewModel.description.content").String()
	}

	// Subscribe state: scan rows/actions for the first subscribeButtonViewModel
	// (YouTube has shuffled these array indexes historically), then look up
	// its stateEntityStoreKey in frameworkUpdates mutations.
	sbvm := findSubscribeButtonViewModel(phvm.Get("actions.flexibleActionsViewModel.actionsRows"))
	if sbvm.Exists() {
		if stateKey := sbvm.Get("stateEntityStoreKey").String(); stateKey != "" {
			data.Get("frameworkUpdates.entityBatchUpdate.mutations").ForEach(func(_, m gjson.Result) bool {
				entity := m.Get("payload.subscriptionStateEntity")
				if entity.Get("key").String() == stateKey {
					d.Subscribed = entity.Get("subscribed").Bool()
					d.SubscribedKnown = true
					return false
				}
				return true
			})
		}
	}

	return d
}

func findSubscribeButtonViewModel(rows gjson.Result) gjson.Result {
	var found gjson.Result
	rows.ForEach(func(_, row gjson.Result) bool {
		row.Get("actions").ForEach(func(_, act gjson.Result) bool {
			if sb := act.Get("subscribeButtonViewModel"); sb.Exists() {
				found = sb
				return false
			}
			return true
		})
		return !found.Exists()
	})
	return found
}
