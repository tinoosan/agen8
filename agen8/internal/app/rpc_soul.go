package app

import (
	"context"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/protocol"
	pkgsoul "github.com/tinoosan/agen8/pkg/services/soul"
)

func registerSoulHandlers(s *RPCServer, reg methodRegistry) error {
	return registerHandlers(
		func() error {
			return addBoundHandler[protocol.SoulGetParams, protocol.SoulGetResult](reg, protocol.MethodSoulGet, true, s.soulGet)
		},
		func() error {
			return addBoundHandler[protocol.SoulUpdateParams, protocol.SoulUpdateResult](reg, protocol.MethodSoulUpdate, false, s.soulUpdate)
		},
		func() error {
			return addBoundHandler[protocol.SoulHistoryParams, protocol.SoulHistoryResult](reg, protocol.MethodSoulHistory, true, s.soulHistory)
		},
	)
}

func toProtocolSoulDoc(doc pkgsoul.Doc) protocol.SoulDoc {
	out := protocol.SoulDoc{
		Content:  strings.TrimSpace(doc.Content),
		Version:  doc.Version,
		Checksum: strings.TrimSpace(doc.Checksum),
		Locked:   doc.Locked,
	}
	if !doc.UpdatedAt.IsZero() {
		out.UpdatedAt = doc.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if actor := strings.TrimSpace(string(doc.UpdatedBy)); actor != "" {
		out.UpdatedBy = actor
	}
	return out
}

func (s *RPCServer) soulGet(ctx context.Context, _ protocol.SoulGetParams) (protocol.SoulGetResult, error) {
	if s == nil || s.soulService == nil {
		return protocol.SoulGetResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "soul service not configured"}
	}
	doc, err := s.soulService.Get(ctx)
	if err != nil {
		return protocol.SoulGetResult{}, err
	}
	return protocol.SoulGetResult{Soul: toProtocolSoulDoc(doc)}, nil
}

func (s *RPCServer) soulUpdate(ctx context.Context, p protocol.SoulUpdateParams) (protocol.SoulUpdateResult, error) {
	if s == nil || s.soulService == nil {
		return protocol.SoulUpdateResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "soul service not configured"}
	}
	doc, err := s.soulService.Update(ctx, pkgsoul.UpdateRequest{
		Content:         p.Content,
		Reason:          p.Reason,
		ExpectedVersion: p.ExpectedVersion,
		OverrideLock:    p.OverrideLock,
		AllowImmutable:  p.AllowImmutable,
		Actor:           pkgsoul.ActorOperator,
	})
	if err != nil {
		return protocol.SoulUpdateResult{}, err
	}
	return protocol.SoulUpdateResult{Soul: toProtocolSoulDoc(doc)}, nil
}

func (s *RPCServer) soulHistory(ctx context.Context, p protocol.SoulHistoryParams) (protocol.SoulHistoryResult, error) {
	if s == nil || s.soulService == nil {
		return protocol.SoulHistoryResult{}, &protocol.ProtocolError{Code: protocol.CodeInvalidState, Message: "soul service not configured"}
	}
	events, next, err := s.soulService.History(ctx, p.Limit, p.Cursor)
	if err != nil {
		return protocol.SoulHistoryResult{}, err
	}
	out := make([]protocol.SoulAuditEvent, 0, len(events))
	for _, ev := range events {
		outEv := protocol.SoulAuditEvent{
			ID:             strings.TrimSpace(ev.ID),
			ActorLayer:     strings.TrimSpace(string(ev.ActorLayer)),
			Action:         strings.TrimSpace(ev.Action),
			Reason:         strings.TrimSpace(ev.Reason),
			VersionBefore:  ev.VersionBefore,
			VersionAfter:   ev.VersionAfter,
			ChecksumBefore: strings.TrimSpace(ev.ChecksumBefore),
			ChecksumAfter:  strings.TrimSpace(ev.ChecksumAfter),
		}
		if !ev.Timestamp.IsZero() {
			outEv.Timestamp = ev.Timestamp.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, outEv)
	}
	return protocol.SoulHistoryResult{Events: out, NextCursor: next}, nil
}
