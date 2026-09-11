package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"zenbot/internal/agent/commandgateway"
	"zenbot/internal/common"
	"zenbot/internal/listener/snapshot"
	"zenbot/internal/model"
	"zenbot/internal/service"
)

// agentCaptureEngine preserves command-specific optional interfaces while
// recording room output for the model observation. Wrapping only common.Engine
// would otherwise erase capabilities such as moderation and prefix control.
type agentCaptureEngine struct {
	common.Engine
	invocationWhisper   bool
	messages            []string
	deliveryCount       int
	actionCount         int
	data                json.RawMessage
	snapshotCompletions []agentSnapshotCompletion
	snapshotStatus      commandgateway.OutcomeStatus
}

type agentSnapshotCompletion struct {
	result  <-chan snapshot.OperationResult
	whisper bool
}

const privateDeliveryObservation = "A private response was delivered to the caller."

func (e *agentCaptureEngine) recordDelivery(message string, whisper bool) {
	if whisper && !e.invocationWhisper {
		message = privateDeliveryObservation
	}
	e.messages = append(e.messages, message)
	e.deliveryCount++
	e.actionCount++
}

func (e *agentCaptureEngine) recordAction(err error) error {
	if err == nil {
		e.actionCount++
	}
	return err
}

func (e *agentCaptureEngine) SubscribeTrip(trip string) bool {
	changed := e.Engine.SubscribeTrip(trip)
	if changed {
		e.actionCount++
	}
	return changed
}

func (e *agentCaptureEngine) UnsubscribeTrip(trip string) bool {
	changed := e.Engine.UnsubscribeTrip(trip)
	if changed {
		e.actionCount++
	}
	return changed
}

func (e *agentCaptureEngine) AddAfkUser(user *model.User, reason string) {
	e.Engine.AddAfkUser(user, reason)
	if user != nil {
		e.actionCount++
	}
}

func requiredAgentCapability[T any](engine common.Engine, name string) (T, error) {
	capability, ok := any(engine).(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%s is unavailable", name)
	}
	return capability, nil
}

func (e *agentCaptureEngine) SendChatMessage(author, message string, whisper bool) (string, error) {
	result, err := e.Engine.SendChatMessage(author, message, whisper)
	if err == nil {
		e.recordDelivery(message, whisper)
	}
	return result, err
}

func (e *agentCaptureEngine) SendWhisperMessage(author, payload string) (string, error) {
	result, err := e.Engine.SendWhisperMessage(author, payload)
	if err == nil {
		e.recordDelivery(payload, true)
	}
	return result, err
}

func (e *agentCaptureEngine) SendAddressedMessage(author, payload string, whisper bool) (string, error) {
	result, err := e.Engine.SendAddressedMessage(author, payload, whisper)
	if err == nil {
		e.recordDelivery(payload, whisper)
	}
	return result, err
}

func (e *agentCaptureEngine) ServiceBundle() *service.Bundle {
	provider, ok := e.Engine.(serviceEngine)
	if !ok {
		return nil
	}
	return provider.ServiceBundle()
}

func (e *agentCaptureEngine) LogCommand(ctx context.Context, record model.CommandAuditRecord) (int64, error) {
	logger, ok := e.Engine.(commandAuditLogger)
	if !ok {
		return 0, nil
	}
	return logger.LogCommand(ctx, record)
}

func (e *agentCaptureEngine) BanNick(ctx context.Context, target common.NickTarget) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.BanNick(ctx, target))
}

func (e *agentCaptureEngine) UnbanHash(ctx context.Context, hash common.BanHash) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.UnbanHash(ctx, hash))
}

func (e *agentCaptureEngine) UnbanAllContext(ctx context.Context) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.UnbanAllContext(ctx))
}

func (e *agentCaptureEngine) LockRoom(ctx context.Context) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.LockRoom(ctx))
}

func (e *agentCaptureEngine) UnlockRoom(ctx context.Context) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.UnlockRoom(ctx))
}

func (e *agentCaptureEngine) EnableCaptcha(ctx context.Context) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.EnableCaptcha(ctx))
}

func (e *agentCaptureEngine) DisableCaptcha(ctx context.Context) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.DisableCaptcha(ctx))
}

func (e *agentCaptureEngine) AuthorizeTrip(ctx context.Context, trip common.Trip) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.AuthorizeTrip(ctx, trip))
}

func (e *agentCaptureEngine) DeauthorizeTrip(ctx context.Context, trip common.Trip) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.DeauthorizeTrip(ctx, trip))
}

func (e *agentCaptureEngine) MuteNick(ctx context.Context, target common.NickTarget) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.MuteNick(ctx, target))
}

func (e *agentCaptureEngine) UnmuteHash(ctx context.Context, hash common.BanHash) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.UnmuteHash(ctx, hash))
}

func (e *agentCaptureEngine) ForceFlair(ctx context.Context, target common.NickTarget, flair common.Flair) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.ForceFlair(ctx, target, flair))
}

func (e *agentCaptureEngine) ForceColor(ctx context.Context, target common.NickTarget, color common.Color) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.ForceColor(ctx, target, color))
}

func (e *agentCaptureEngine) KickNick(ctx context.Context, target common.NickTarget) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.KickNick(ctx, target))
}

func (e *agentCaptureEngine) KickNickTo(ctx context.Context, target common.NickTarget, channel common.Channel) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.KickNickTo(ctx, target, channel))
}

func (e *agentCaptureEngine) OverflowNick(ctx context.Context, target common.NickTarget) error {
	operations, err := requiredAgentCapability[common.ModerationOperations](e.Engine, "moderation operations")
	if err != nil {
		return err
	}
	return e.recordAction(operations.OverflowNick(ctx, target))
}

func (e *agentCaptureEngine) SubmitRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	submitter, err := requiredAgentCapability[common.RoomSnapshotSubmitter](e.Engine, "room snapshot submitter")
	if err != nil {
		return err
	}
	return e.submitSnapshot(request, submitter.SubmitRoomSnapshot)
}

func (e *agentCaptureEngine) SubmitCredentialedRoomSnapshot(request snapshot.RoomSnapshotRequest) error {
	submitter, err := requiredAgentCapability[common.CredentialedRoomSnapshotSubmitter](e.Engine, "credentialed room snapshot submitter")
	if err != nil {
		return err
	}
	return e.submitSnapshot(request, submitter.SubmitCredentialedRoomSnapshot)
}

func (e *agentCaptureEngine) submitSnapshot(request snapshot.RoomSnapshotRequest, submit func(snapshot.RoomSnapshotRequest) error) error {
	completed := make(chan snapshot.OperationResult, 1)
	previous := request.OnComplete
	request.OnComplete = func(result snapshot.OperationResult) {
		captured := result
		captured.Data = append(json.RawMessage(nil), result.Data...)
		select {
		case completed <- captured:
		default:
		}
		if previous != nil {
			previous(result)
		}
	}
	if err := submit(request); err != nil {
		select {
		case result := <-completed:
			completed <- result
			e.snapshotCompletions = append(e.snapshotCompletions, agentSnapshotCompletion{result: completed, whisper: request.Whisper})
		default:
		}
		return err
	}
	e.snapshotCompletions = append(e.snapshotCompletions, agentSnapshotCompletion{result: completed, whisper: request.Whisper})
	return nil
}

func (e *agentCaptureEngine) awaitSnapshotCompletions(ctx context.Context) error {
	var completionErr error
	for _, completion := range e.snapshotCompletions {
		// Cancellation belongs to the execution owner. Wait for its terminal
		// receipt, including operation return and cleanup, even after ctx expires.
		result := <-completion.result
		e.actionCount += result.ActionCount
		e.deliveryCount += result.DeliveryCount
		if result.OutcomeUnknown {
			e.snapshotStatus = commandgateway.OutcomeUnknown
		} else if e.snapshotStatus != commandgateway.OutcomeUnknown {
			switch result.Outcome {
			case snapshot.OutcomeAbsentTarget:
				e.snapshotStatus = commandgateway.OutcomeNotFound
			case snapshot.OutcomeEmpty, snapshot.OutcomeSkipped, snapshot.OutcomeFailed:
				e.snapshotStatus = commandgateway.OutcomeRejected
			case snapshot.OutcomeSuccess:
				if e.snapshotStatus == "" {
					e.snapshotStatus = commandgateway.OutcomeSucceeded
				}
			}
		}
		message := strings.TrimSpace(result.Reply)
		if result.Outcome == snapshot.OutcomeFailed {
			if message == "" {
				message = "remote room operation failed"
			}
			operationErr := result.Error
			if operationErr == nil {
				operationErr = fmt.Errorf("%s", message)
			}
			completionErr = errors.Join(completionErr, operationErr)
		}
		if result.Outcome == snapshot.OutcomeSuccess && len(result.Data) > 0 && (!completion.whisper || e.invocationWhisper) {
			e.data = append(json.RawMessage(nil), result.Data...)
		}
		if message != "" && result.Outcome != snapshot.OutcomeFailed && result.DeliveryCount > 0 {
			if completion.whisper && !e.invocationWhisper {
				message = privateDeliveryObservation
			}
			e.messages = append(e.messages, message)
		}
	}
	return completionErr
}

func (e *agentCaptureEngine) UpdatePrefix(prefix string) (string, error) {
	controller, err := requiredAgentCapability[common.PrefixController](e.Engine, "prefix controller")
	if err != nil {
		return "", err
	}
	previous, err := controller.UpdatePrefix(prefix)
	return previous, e.recordAction(err)
}

func (e *agentCaptureEngine) AutoMoveSnapshot() common.AutoMoveSnapshot {
	controller, ok := e.Engine.(common.AutoMoveController)
	if !ok {
		return common.AutoMoveSnapshot{}
	}
	return controller.AutoMoveSnapshot()
}

func (e *agentCaptureEngine) ConfigureAutoMove(source, destination string) (common.AutoMoveSnapshot, error) {
	controller, err := requiredAgentCapability[common.AutoMoveController](e.Engine, "auto-move controller")
	if err != nil {
		return common.AutoMoveSnapshot{}, err
	}
	state, err := controller.ConfigureAutoMove(source, destination)
	return state, e.recordAction(err)
}

func (e *agentCaptureEngine) EnableAutoMove(ctx context.Context) (common.AutoMoveSnapshot, error) {
	controller, err := requiredAgentCapability[common.AutoMoveController](e.Engine, "auto-move controller")
	if err != nil {
		return common.AutoMoveSnapshot{}, err
	}
	state, err := controller.EnableAutoMove(ctx)
	return state, e.recordAction(err)
}

func (e *agentCaptureEngine) DisableAutoMove(ctx context.Context) (common.AutoMoveSnapshot, error) {
	controller, err := requiredAgentCapability[common.AutoMoveController](e.Engine, "auto-move controller")
	if err != nil {
		return common.AutoMoveSnapshot{}, err
	}
	state, err := controller.DisableAutoMove(ctx)
	return state, e.recordAction(err)
}

func (e *agentCaptureEngine) MoveFromServingRoom(ctx context.Context, source string, target common.NickTarget, destination common.Channel) (bool, error) {
	mover, err := requiredAgentCapability[common.LiveRoomMover](e.Engine, "live room mover")
	if err != nil {
		return false, err
	}
	moved, err := mover.MoveFromServingRoom(ctx, source, target, destination)
	if moved && err == nil {
		e.recordAction(nil)
	}
	return moved, err
}

func (e *agentCaptureEngine) AddReplica(ctx context.Context, channel string) error {
	controller, err := requiredAgentCapability[ReplicaController](e.Engine, "replica controller")
	if err != nil {
		return err
	}
	return e.recordAction(controller.AddReplica(ctx, channel))
}

func (e *agentCaptureEngine) RemoveReplica(ctx context.Context, channel string) error {
	controller, err := requiredAgentCapability[ReplicaController](e.Engine, "replica controller")
	if err != nil {
		return err
	}
	return e.recordAction(controller.RemoveReplica(ctx, channel))
}

func (e *agentCaptureEngine) ReplicaChannels() []string {
	controller, ok := e.Engine.(ReplicaController)
	if !ok {
		return nil
	}
	return append([]string(nil), controller.ReplicaChannels()...)
}

func (e *agentCaptureEngine) HostLifecycleController() common.HostLifecycleController {
	provider, ok := e.Engine.(lifecycleControllerProvider)
	if !ok {
		return nil
	}
	return provider.HostLifecycleController()
}
