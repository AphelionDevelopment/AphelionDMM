package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SpaiR/imgui-go"
	"github.com/rs/zerolog/log"

	"sdmm/internal/aphelion/collab/authority"
	"sdmm/internal/aphelion/collab/client"
	"sdmm/internal/aphelion/collab/identity"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relayclient"
	"sdmm/internal/aphelion/collab/replica"
	"sdmm/internal/aphelion/collab/store"
	collabui "sdmm/internal/aphelion/collab/ui"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/editor"
	dial "sdmm/internal/app/ui/dialog"
	"sdmm/internal/app/window"
	w "sdmm/internal/imguiext/widget"
	"sdmm/internal/platform"
	"sdmm/internal/util"
)

type onlineCollaboration struct {
	mutex               sync.Mutex
	ctx                 context.Context
	cancel              context.CancelFunc
	transport           *relayclient.Client
	document            *authority.Document
	authority           *authority.OwnerSession
	replica             *replica.Session
	participantExecutor *relayclient.ParticipantExecutor
	groupKey            protocolv2.GroupKey
	relayURL            string
	sessionID           string
	displayName         string
	role                protocolv2.Role
	documentID          model.DocumentID
	environmentHash     string
	admissionGeneration uint64
	profileSequence     uint64
	terminalError       error
	reconnecting        bool
	invitation          *protocolv2.Invitation
}

func (a *app) DoCreateOnlineCollaborationSession() {
	selectedEditor := a.CurrentEditor()
	if selectedEditor == nil || !a.OnlineCollaborationAvailable() || a.HasActiveCollaboration() {
		return
	}
	displayName := "Owner"
	relayURL := a.collaborationConfig().RelayURL
	dial.Open(dial.TypeCustom{
		Title:       "Start Online Collaboration Session",
		CloseButton: true,
		Layout: w.Layout{
			w.Text("Choose your display name and relay endpoint."),
			w.InputTextWithHint("##online-collaboration-owner-name", "Display name", &displayName).Width(-1),
			w.InputTextWithHint("##online-collaboration-relay", "https://mapping.example", &relayURL).Width(-1),
			w.Button("Start Session", func() {
				name := strings.TrimSpace(displayName)
				endpoint := strings.TrimSpace(relayURL)
				if name == "" || relayclient.ValidateEndpoint(endpoint) != nil {
					util.ShowErrorDialog("Unable to start online collaboration: display name or relay endpoint is invalid")
					return
				}
				if a.CurrentEditor() != selectedEditor || a.HasActiveCollaboration() {
					util.ShowErrorDialog("Unable to start online collaboration: the active map or session changed")
					return
				}
				imgui.CloseCurrentPopup()
				go a.startOnlineCollaboration(selectedEditor, name, endpoint)
			}),
		},
	})
}

func (a *app) startOnlineCollaboration(selectedEditor *editor.Editor, displayName, relayURL string) {
	ctx, cancel := context.WithCancel(context.Background())
	fail := func(err error) {
		cancel()
		window.RunLater(func() { util.ShowErrorDialog("Unable to start online collaboration: " + err.Error()) })
	}
	snapshot, err := selectedEditor.CollaborationSnapshot(ctx)
	if err != nil {
		fail(err)
		return
	}
	var roomID protocolv2.RoomID
	if _, err := rand.Read(roomID[:]); err != nil {
		fail(fmt.Errorf("generate room identity: %w", err))
		return
	}
	groupKey, groupReference, err := a.clientCollaboration.manager.CreateSessionKey(ctx, identity.GroupKeyNamespace)
	if err != nil {
		fail(err)
		return
	}
	manifest := protocolv2.RoleManifest{
		RoomID: roomID, Generation: 1, OwnerKey: a.clientCollaboration.identity.PublicKey, GroupKeyGeneration: 1,
		Members: []protocolv2.Member{{ActorKey: a.clientCollaboration.identity.PublicKey, ActorID: selectedEditor.CollaborationActorID(), Role: protocolv2.RoleOwner, DisplayName: displayName}},
	}
	signedManifest, err := authority.SignManifest(manifest, a.clientCollaboration.identity.PrivateKey)
	if err != nil {
		fail(err)
		return
	}
	document, err := authority.StartDocument(ctx, snapshot, a.clientCollaboration.store)
	if err != nil {
		fail(err)
		return
	}
	ownerAuthority, err := authority.NewOwnerSession(document, a.clientCollaboration.store, a.clientCollaboration.identity.PrivateKey, signedManifest)
	if err != nil {
		_ = document.Close(context.Background())
		fail(err)
		return
	}
	transport, err := relayclient.ConnectOwner(ctx, relayURL, roomID, a.clientCollaboration.identity.PrivateKey)
	if err != nil {
		_ = document.Close(context.Background())
		fail(err)
		return
	}
	execution, err := relayclient.NewOwnerExecutor(transport, ownerAuthority, groupKey)
	if err != nil {
		_ = transport.Close()
		_ = document.Close(context.Background())
		fail(err)
		return
	}
	manifestBytes, err := json.Marshal(signedManifest)
	if err != nil {
		_ = transport.Close()
		_ = document.Close(context.Background())
		fail(err)
		return
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		_ = transport.Close()
		_ = document.Close(context.Background())
		fail(err)
		return
	}
	sessionID := hex.EncodeToString(roomID[:])
	local := store.LocalSession{
		SessionID: sessionID, RelayURL: relayURL, Role: protocolv2.RoleOwner, DocumentID: snapshot.DocumentID,
		OwnerPublicKey: manifest.OwnerKey, Manifest: manifestBytes, ManifestSHA256: sha256.Sum256(manifestBytes),
		AcknowledgedRevision: snapshot.Revision, AcknowledgedMapHash: mapHash, DisplayName: displayName,
		IdentitySecretRef: a.collaborationConfig().IdentitySecretRef, GroupKeySecretRef: string(groupReference), OwnerSecretRef: a.collaborationConfig().IdentitySecretRef,
	}
	if err := a.clientCollaboration.store.CreateClientSession(ctx, local); err != nil {
		_ = transport.Close()
		_ = document.Close(context.Background())
		fail(err)
		return
	}
	online := &onlineCollaboration{ctx: ctx, cancel: cancel, transport: transport, document: document, authority: ownerAuthority, groupKey: groupKey, relayURL: relayURL, sessionID: sessionID, displayName: displayName, role: protocolv2.RoleOwner, documentID: snapshot.DocumentID, environmentHash: snapshot.EnvironmentHash}
	window.RunLater(func() {
		if a.CurrentEditor() != selectedEditor || a.HasActiveCollaboration() {
			online.close()
			return
		}
		if err := selectedEditor.AttachCollaborationExecutor(execution); err != nil {
			online.close()
			util.ShowErrorDialog("Unable to attach online collaboration: " + err.Error())
			return
		}
		a.onlineCollaboration = online
		a.collaborationEditor = selectedEditor
		go online.serveOwner()
	})
}

func (a *app) joinOnlineCollaboration(selectedEditor *editor.Editor, invitation protocolv2.Invitation, displayName string) {
	ctx, cancel := context.WithCancel(context.Background())
	fail := func(err error) {
		cancel()
		window.RunLater(func() { util.ShowErrorDialog("Unable to join online collaboration: " + err.Error()) })
	}
	localSnapshot, err := selectedEditor.CollaborationSnapshot(ctx)
	if err != nil {
		fail(err)
		return
	}
	if localSnapshot.EnvironmentHash != invitation.EnvironmentHash {
		fail(fmt.Errorf("the invitation uses a different project environment"))
		return
	}
	localSnapshot.DocumentID = invitation.DocumentID
	mapHash, err := localSnapshot.Hash()
	if err != nil {
		fail(err)
		return
	}
	groupReference, err := a.clientCollaboration.manager.StoreSessionKey(ctx, identity.GroupKeyNamespace, [32]byte(invitation.GroupKey))
	if err != nil {
		fail(err)
		return
	}
	transport, err := relayclient.ConnectParticipant(ctx, invitation.RelayURL, invitation.RoomID, a.clientCollaboration.identity.PrivateKey, [32]byte(invitation.Admission))
	if err != nil {
		fail(err)
		return
	}
	manifestBytes, err := json.Marshal(invitation)
	if err != nil {
		_ = transport.Close()
		fail(err)
		return
	}
	if err := a.clientCollaboration.store.Create(ctx, localSnapshot); err != nil {
		_ = transport.Close()
		fail(err)
		return
	}
	sessionID := hex.EncodeToString(invitation.RoomID[:])
	local := store.LocalSession{
		SessionID: sessionID, RelayURL: invitation.RelayURL, Role: invitation.Role, DocumentID: invitation.DocumentID,
		OwnerPublicKey: invitation.OwnerPublicKey, Manifest: manifestBytes, ManifestSHA256: sha256.Sum256(manifestBytes),
		AcknowledgedRevision: localSnapshot.Revision, AcknowledgedMapHash: mapHash, DisplayName: displayName,
		IdentitySecretRef: a.collaborationConfig().IdentitySecretRef, GroupKeySecretRef: string(groupReference),
	}
	if err := a.clientCollaboration.store.CreateClientSession(ctx, local); err != nil {
		_ = transport.Close()
		fail(err)
		return
	}
	replicaSession, err := replica.NewSession(a.clientCollaboration.store, local)
	if err != nil {
		_ = transport.Close()
		fail(err)
		return
	}
	participant := &relayclient.Participant{
		Transport: transport, Replica: replicaSession, GroupKey: invitation.GroupKey,
		ActorID: selectedEditor.CollaborationActorID(), DisplayName: displayName, Invitation: &invitation,
	}
	if _, err := participant.Synchronize(ctx, invitation.EnvironmentHash, nil); err != nil {
		_ = transport.Close()
		fail(err)
		return
	}
	execution, err := relayclient.NewParticipantExecutor(participant, selectedEditor.CollaborationActorID())
	if err != nil {
		_ = transport.Close()
		fail(err)
		return
	}
	online := &onlineCollaboration{
		ctx: ctx, cancel: cancel, transport: transport, replica: replicaSession, participantExecutor: execution,
		groupKey: invitation.GroupKey, relayURL: invitation.RelayURL, sessionID: sessionID, displayName: displayName,
		role: invitation.Role, documentID: invitation.DocumentID, environmentHash: invitation.EnvironmentHash, invitation: &invitation,
	}
	window.RunLater(func() {
		if a.CurrentEditor() != selectedEditor || a.HasActiveCollaboration() {
			online.close()
			return
		}
		if err := selectedEditor.AttachCollaborationExecutor(execution); err != nil {
			online.close()
			util.ShowErrorDialog("Unable to attach online collaboration: " + err.Error())
			return
		}
		a.onlineCollaboration = online
		a.collaborationEditor = selectedEditor
		execution.Start(ctx)
	})
}

func (a *app) DoOpenCollaborationSettings() {
	if a.HasActiveCollaboration() {
		return
	}
	relayURL := a.collaborationConfig().RelayURL
	dial.Open(dial.TypeCustom{Title: "Collaboration Settings", CloseButton: true, Layout: w.Layout{
		w.Text("Default public relay endpoint"),
		w.InputTextWithHint("##collaboration-settings-relay", "https://mapping.example", &relayURL).Width(-1),
		w.Button("Save", func() {
			endpoint := strings.TrimSpace(relayURL)
			if relayclient.ValidateEndpoint(endpoint) != nil {
				util.ShowErrorDialog("Unable to save collaboration settings: relay endpoint is invalid")
				return
			}
			a.collaborationConfig().RelayURL = endpoint
			a.configSaveV(a.collaborationConfig())
			imgui.CloseCurrentPopup()
		}),
	}})
}

func (a *app) createOnlineInvitation(role collabui.InvitationRole) error {
	online := a.onlineCollaboration
	if online == nil {
		return fmt.Errorf("online collaboration is not active")
	}
	if online.authority == nil || online.role != protocolv2.RoleOwner {
		return fmt.Errorf("only the online session owner can create invitations")
	}
	protocolRole := protocolv2.Role(role)
	if protocolRole != protocolv2.RoleEditor && protocolRole != protocolv2.RoleViewer {
		return fmt.Errorf("invitation role is invalid")
	}
	online.mutex.Lock()
	defer online.mutex.Unlock()
	capability, _, err := a.clientCollaboration.manager.CreateSessionKey(context.Background(), identity.CapabilityNamespace)
	if err != nil {
		return err
	}
	manifest := online.authority.Manifest()
	manifestDigest, err := authority.DigestManifest(manifest)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	unsigned := protocolv2.Invitation{
		Version: protocolv2.Version, RelayURL: online.relayURL, RoomID: manifest.RoomID, Role: protocolRole,
		Admission: protocolv2.Capability(capability), GroupKey: online.groupKey, OwnerPublicKey: manifest.OwnerKey,
		DocumentID: online.documentID, EnvironmentHash: online.environmentHash,
		ManifestSHA256: manifestDigest, IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}
	encoded, err := protocolv2.EncodeInvitation(unsigned, a.clientCollaboration.identity.PrivateKey)
	if err != nil {
		return err
	}
	invitation, err := protocolv2.ParseInvitation(encoded, now)
	if err != nil {
		return err
	}
	if err := online.authority.RegisterInvitation(invitation, now); err != nil {
		return err
	}
	online.admissionGeneration++
	if err := online.transport.ReplaceAdmissions(context.Background(), online.admissionGeneration, online.authority.RelayAdmissions()); err != nil {
		return err
	}
	admissions := make([]store.Admission, 0)
	for _, admission := range online.authority.RelayAdmissions() {
		admissions = append(admissions, store.Admission{SessionID: online.sessionID, CapabilityID: admission.Digest, Role: admission.Role, ExpiresAt: admission.ExpiresAt, BoundActor: admission.BoundActor})
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := a.clientCollaboration.store.SaveManifest(context.Background(), online.sessionID, manifestBytes, admissions); err != nil {
		return err
	}
	platform.SetClipboard(encoded)
	return nil
}

func (a *app) stopOnlineCollaboration() {
	if a.onlineCollaboration == nil {
		return
	}
	a.onlineCollaboration.close()
	a.onlineCollaboration = nil
}

func (a *app) retryOnlineCollaboration() {
	online := a.onlineCollaboration
	editor := a.collaborationEditor
	if online == nil || editor == nil {
		return
	}
	online.mutex.Lock()
	if online.terminalError == nil || online.reconnecting {
		online.mutex.Unlock()
		return
	}
	online.reconnecting = true
	online.mutex.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(online.ctx, 15*time.Second)
		defer cancel()
		var (
			transport *relayclient.Client
			execution any
			err       error
		)
		if online.authority != nil {
			transport, err = relayclient.ConnectOwner(ctx, online.relayURL, online.authority.Manifest().RoomID, a.clientCollaboration.identity.PrivateKey)
			if err == nil && online.admissionGeneration != 0 {
				err = transport.ReplaceAdmissions(ctx, online.admissionGeneration, online.authority.RelayAdmissions())
			}
			if err == nil {
				execution, err = relayclient.NewOwnerExecutor(transport, online.authority, online.groupKey)
			}
		} else if online.invitation != nil {
			transport, err = relayclient.ConnectParticipant(ctx, online.relayURL, online.invitation.RoomID, a.clientCollaboration.identity.PrivateKey, [32]byte(online.invitation.Admission))
			if err == nil {
				participant := &relayclient.Participant{Transport: transport, Replica: online.replica, GroupKey: online.groupKey, ActorID: editor.CollaborationActorID(), DisplayName: online.displayName, Invitation: online.invitation}
				pending, pendingErr := a.clientCollaboration.store.ListPending(ctx, online.sessionID)
				pendingIDs := make([]model.OperationID, 0, len(pending))
				for _, item := range pending {
					pendingIDs = append(pendingIDs, item.Operation.OperationID)
				}
				if pendingErr != nil {
					err = pendingErr
				} else if _, err = participant.Synchronize(ctx, online.environmentHash, pendingIDs); err == nil {
					execution, err = relayclient.NewParticipantExecutor(participant, editor.CollaborationActorID())
				}
			}
		} else {
			err = fmt.Errorf("online collaboration recovery data is unavailable")
		}
		if err != nil {
			if transport != nil {
				_ = transport.Close()
			}
			online.mutex.Lock()
			online.reconnecting = false
			online.mutex.Unlock()
			window.RunLater(func() { util.ShowErrorDialog("Unable to reconnect collaboration: " + err.Error()) })
			return
		}
		window.RunLater(func() {
			if a.onlineCollaboration != online || a.collaborationEditor != editor {
				_ = transport.Close()
				online.mutex.Lock()
				online.reconnecting = false
				online.mutex.Unlock()
				return
			}
			switch recovered := execution.(type) {
			case *relayclient.OwnerExecutor:
				if err := editor.AttachCollaborationExecutor(recovered); err != nil {
					_ = transport.Close()
					online.mutex.Lock()
					online.reconnecting = false
					online.mutex.Unlock()
					util.ShowErrorDialog("Unable to reconnect collaboration: " + err.Error())
					return
				}
				online.mutex.Lock()
				online.transport = transport
				online.terminalError = nil
				online.reconnecting = false
				online.mutex.Unlock()
				go online.serveOwner()
			case *relayclient.ParticipantExecutor:
				if err := editor.AttachCollaborationExecutor(recovered); err != nil {
					_ = transport.Close()
					online.mutex.Lock()
					online.reconnecting = false
					online.mutex.Unlock()
					util.ShowErrorDialog("Unable to reconnect collaboration: " + err.Error())
					return
				}
				online.mutex.Lock()
				online.transport = transport
				online.participantExecutor = recovered
				online.terminalError = nil
				online.reconnecting = false
				online.mutex.Unlock()
				recovered.Start(online.ctx)
			}
		})
	}()
}

func (online *onlineCollaboration) serveOwner() {
	err := (&relayclient.Owner{Transport: online.transport, Authority: online.authority, GroupKey: online.groupKey}).Serve(online.ctx)
	if online.ctx.Err() == nil {
		online.mutex.Lock()
		online.terminalError = err
		online.mutex.Unlock()
		log.Error().Err(err).Msg("online collaboration owner transport stopped")
	}
}

func (a *app) updateOnlineDisplayName(displayName string) error {
	online := a.onlineCollaboration
	if online == nil {
		return fmt.Errorf("online collaboration is not active")
	}
	online.mutex.Lock()
	defer online.mutex.Unlock()
	online.profileSequence++
	if online.authority == nil {
		if displayName == "" {
			online.profileSequence--
			return fmt.Errorf("display name is invalid")
		}
		if err := online.transport.SendApplication(context.Background(), online.groupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationProfileUpdate, protocolv2.ProfileUpdate{DisplayName: displayName, Sequence: online.profileSequence}); err != nil {
			online.profileSequence--
			return err
		}
		online.displayName = displayName
		return nil
	}
	response, err := online.authority.Handle(context.Background(), online.transport.ActorKey(), protocolv2.DecodedApplication{
		Type:    protocolv2.ApplicationProfileUpdate,
		Payload: &protocolv2.ProfileUpdate{DisplayName: displayName, Sequence: online.profileSequence},
	})
	if err != nil {
		online.profileSequence--
		return err
	}
	if err := online.transport.SendApplication(context.Background(), online.groupKey, protocolv2.RouteRoom, protocolv2.ActorKey{}, response.Type, response.Payload); err != nil {
		return err
	}
	online.displayName = displayName
	return nil
}

func (online *onlineCollaboration) close() {
	online.cancel()
	_ = online.transport.Close()
	if online.document != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = online.document.Close(ctx)
	}
}

func (a *app) OnlineCollaborationAvailable() bool {
	return a.clientCollaboration != nil
}

func (a *app) onlineCollaborationViewModel() collabui.ViewModel {
	online := a.onlineCollaboration
	if online == nil {
		return collabui.ViewModel{}
	}
	online.mutex.Lock()
	defer online.mutex.Unlock()
	var snapshot model.Snapshot
	if online.authority != nil {
		snapshot, _ = online.authority.Snapshot(context.Background())
	} else if online.participantExecutor != nil {
		snapshot, _ = online.participantExecutor.Snapshot(context.Background())
		if online.terminalError == nil {
			online.terminalError = online.participantExecutor.TerminalError()
		}
	}
	state := client.StateCaughtUp
	if online.role == protocolv2.RoleViewer {
		state = client.StateReadOnly
	}
	status := collabui.SessionStatus{SessionID: online.sessionID, Role: string(online.role), State: state, Revision: snapshot.Revision, InviteReady: online.role == protocolv2.RoleOwner, Err: online.terminalError}
	if online.terminalError != nil {
		status.State = client.StateReconnecting
		status.Paused = true
		status.ReconnectReady = !online.reconnecting
	}
	return collabui.BuildViewModel(status)
}
