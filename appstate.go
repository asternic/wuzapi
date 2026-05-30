package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waEvents "go.mau.fi/whatsmeow/types/events"
)

func sendAppStatePatch(ctx context.Context, client *whatsmeow.Client, buildPatch func() appstate.PatchInfo) error {
	patch := buildPatch()
	err := client.SendAppState(ctx, patch)
	if err == nil || !isRecoverableAppStateError(err) {
		return err
	}

	for attempt := 1; attempt <= 2; attempt++ {
		log.Warn().
			Err(err).
			Str("appStateType", string(patch.Type)).
			Int("attempt", attempt).
			Msg("App state update failed, fetching a full app state sync before retry")

		if syncErr := recoverAppState(ctx, client, patch.Type); syncErr != nil {
			return fmt.Errorf("%w (also, failed to recover app state %s: %w)", err, patch.Type, syncErr)
		}
		if sleepErr := sleepWithContext(ctx, time.Duration(attempt)*time.Second); sleepErr != nil {
			return fmt.Errorf("%w (also, retry was canceled: %w)", err, sleepErr)
		}

		patch = buildPatch()
		err = client.SendAppState(ctx, patch)
		if err == nil || !isRecoverableAppStateError(err) {
			return err
		}
	}

	return err
}

func recoverAppState(ctx context.Context, client *whatsmeow.Client, patchType appstate.WAPatchName) error {
	if err := fetchAppStateAfterKeySync(ctx, client, patchType); err == nil {
		return nil
	} else if !isRecoverableAppStateError(err) {
		return err
	} else if recoveryErr := requestAppStateRecovery(ctx, client, patchType); recoveryErr != nil {
		return fmt.Errorf("%w (also, peer recovery failed: %w)", err, recoveryErr)
	}

	return nil
}

func fetchAppStateAfterKeySync(ctx context.Context, client *whatsmeow.Client, patchType appstate.WAPatchName) error {
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	synced := make(chan error, 1)
	handlerID := client.AddEventHandler(func(evt any) {
		switch event := evt.(type) {
		case *waEvents.AppStateSyncComplete:
			if event.Name == patchType {
				notifyRecoveryResult(synced, nil)
			}
		case *waEvents.AppStateSyncError:
			if event.Name == patchType && !isMissingAppStateKeyError(event.Error) {
				notifyRecoveryResult(synced, event.Error)
			}
		}
	})
	defer client.RemoveEventHandler(handlerID)

	err := client.FetchAppState(ctx, patchType, true, false)
	if err == nil || !isMissingAppStateKeyError(err) {
		return err
	}

	log.Warn().
		Err(err).
		Str("appStateType", string(patchType)).
		Msg("Waiting for app state keys from the primary device")

	select {
	case syncErr := <-synced:
		return syncErr
	case <-waitCtx.Done():
		return fmt.Errorf("%w (also, app state keys were not received: %w)", err, waitCtx.Err())
	}
}

func requestAppStateRecovery(ctx context.Context, client *whatsmeow.Client, patchType appstate.WAPatchName) error {
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	recovered := make(chan error, 1)
	handlerID := client.AddEventHandler(func(evt any) {
		switch event := evt.(type) {
		case *waEvents.AppStateSyncComplete:
			if event.Name == patchType && event.Recovery {
				notifyRecoveryResult(recovered, nil)
			}
		case *waEvents.AppStateSyncError:
			if event.Name == patchType && !isRecoverableAppStateError(event.Error) {
				notifyRecoveryResult(recovered, event.Error)
			}
		}
	})
	defer client.RemoveEventHandler(handlerID)

	log.Warn().
		Str("appStateType", string(patchType)).
		Msg("Requesting app state recovery from the primary device")
	if _, err := client.SendPeerMessage(ctx, whatsmeow.BuildAppStateRecoveryRequest(patchType)); err != nil {
		return fmt.Errorf("failed to request app state recovery: %w", err)
	}

	select {
	case err := <-recovered:
		return err
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

func notifyRecoveryResult(recovered chan<- error, err error) {
	select {
	case recovered <- err:
	default:
	}
}

func isRecoverableAppStateError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, appstate.ErrMismatchingLTHash) {
		return true
	}
	if isMissingAppStateKeyError(err) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "mismatching lthash") ||
		strings.Contains(message, `code="409"`) ||
		strings.Contains(message, "conflict")
}

func isMissingAppStateKeyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, appstate.ErrKeyNotFound) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no app state keys found") ||
		strings.Contains(message, "didn't find app state key")
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
