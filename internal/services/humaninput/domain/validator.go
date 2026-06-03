package domain

import (
	"encoding/json"
	"fmt"
)

type PrimitiveValidator interface {
	ValidateDeclaration(Declaration) error
	ValidateResult(Declaration, json.RawMessage) error
}

type DefaultValidator struct{}

func (DefaultValidator) ValidateDeclaration(declaration Declaration) error {
	if err := declaration.Validate(); err != nil {
		return err
	}
	switch declaration.Kind {
	case PrimitiveQuestions:
		var payload QuestionsPayload
		if err := json.Unmarshal(declaration.Payload, &payload); err != nil {
			return fmt.Errorf("human input questions payload is invalid: %w", err)
		}
		if len(payload.Questions) == 0 {
			return fmt.Errorf("human input questions payload requires at least one question")
		}
	case PrimitiveApproveReject:
		var payload ApproveRejectPayload
		if err := json.Unmarshal(declaration.Payload, &payload); err != nil {
			return fmt.Errorf("human input approve/reject payload is invalid: %w", err)
		}
		if payload.Title == "" {
			return fmt.Errorf("human input approve/reject title is required")
		}
	case PrimitiveConfirm, PrimitiveForm:
		if !json.Valid(declaration.Payload) {
			return fmt.Errorf("human input payload must be valid JSON")
		}
	default:
		return declaration.Validate()
	}
	return nil
}

func (DefaultValidator) ValidateResult(declaration Declaration, result json.RawMessage) error {
	if len(result) == 0 || !json.Valid(result) {
		return fmt.Errorf("human input result must be valid JSON")
	}
	switch declaration.Kind {
	case PrimitiveQuestions:
		var decoded QuestionsResult
		if err := json.Unmarshal(result, &decoded); err != nil {
			return fmt.Errorf("human input questions result is invalid: %w", err)
		}
		if !decoded.Cancelled && len(decoded.Answers) == 0 {
			return fmt.Errorf("human input questions result requires answers or cancelled=true")
		}
	case PrimitiveApproveReject:
		var decoded ApproveRejectResult
		if err := json.Unmarshal(result, &decoded); err != nil {
			return fmt.Errorf("human input approve/reject result is invalid: %w", err)
		}
		if !decoded.Cancelled && decoded.Decision == "" {
			return fmt.Errorf("human input approve/reject result requires decision or cancelled=true")
		}
	case PrimitiveConfirm, PrimitiveForm:
		if !json.Valid(result) {
			return fmt.Errorf("human input result must be valid JSON")
		}
	default:
		return declaration.Validate()
	}
	return nil
}
