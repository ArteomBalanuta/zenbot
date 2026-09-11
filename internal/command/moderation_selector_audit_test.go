package command

import (
	"context"
	"errors"
	"testing"

	"zenbot/internal/common"
	"zenbot/internal/model"
	"zenbot/internal/repository"
)

type moderationSelectorKickEngine struct {
	*commandEngineStub
	errors []error
	cancel context.CancelFunc
}

func (e *moderationSelectorKickEngine) KickNick(_ context.Context, target common.NickTarget) error {
	e.raws = append(e.raws, `{"cmd":"kick","nick":"`+string(target)+`"}`)
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	if index := len(e.raws) - 1; index < len(e.errors) {
		return e.errors[index]
	}
	return nil
}

func TestModerationSelectorAuditUnshadowRejectsMixedAllBeforeDeletion(t *testing.T) {
	for _, text := range []string{"!unshadowban alice -all", "!unshadowban -all alice", "!unshadowban -all -all", "!unshadowban alice bob"} {
		t.Run(text, func(t *testing.T) {
			repo := &shadowBanRepositoryStub{records: []repository.ShadowBanRecord{{Name: "alice"}, {Name: "unrelated"}}}
			engine := newShadowBanEngine(repo, nil)
			definition, _ := commandDefinitionFor("unshadowban")
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: text}).Execute(context.Background())
			if repo.removeAlls != 0 || repo.removedTarget != "" || status != model.FAILED {
				t.Fatalf("malformed selector mutated storage: all=%d target=%q status=%v err=%v", repo.removeAlls, repo.removedTarget, status, err)
			}
		})
	}
}

func TestModerationSelectorAuditShadowRejectsMalformedModeBeforeMutation(t *testing.T) {
	for _, text := range []string{"!shadowban -c", "!shadowban -c -c", "!shadowban alice -c", "!shadowban -c alice extra", "!shadowban alice bob"} {
		t.Run(text, func(t *testing.T) {
			repo := &shadowBanRepositoryStub{}
			engine := newShadowBanEngine(repo, map[string]*model.User{"alice": {Name: "alice", Trip: "trip"}})
			definition, _ := commandDefinitionFor("shadowban")
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: text}).Execute(context.Background())
			if len(repo.persisted) != 0 || len(engine.kicked) != 0 || status != model.FAILED {
				t.Fatalf("malformed selector executed: writes=%v kicks=%v status=%v err=%v", repo.persisted, engine.kicked, status, err)
			}
		})
	}
}

func TestModerationSelectorAuditKickValidatesCompleteSelectionBeforeSending(t *testing.T) {
	for _, text := range []string{"!kick -m", "!kick -c", "!kick alice bob", "!kick -c -m", "!kick -c alice extra", "!kick -m alice -c bob", "!kick -m alice -m bob", "!kick -m alice @"} {
		t.Run(text, func(t *testing.T) {
			engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "alice"}}}
			definition, _ := commandDefinitionFor("kick")
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: text}).Execute(context.Background())
			if len(engine.raws) != 0 || status != model.FAILED {
				t.Fatalf("malformed selection sent action: frames=%v status=%v err=%v", engine.raws, status, err)
			}
		})
	}
}

func TestModerationSelectorAuditKickSendsOncePerResolvedNick(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{"alice": {Name: "Alice"}}}
	definition, _ := commandDefinitionFor("kick")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!kick -m alice @ALICE Alice"}).Execute(context.Background())
	if len(engine.raws) != 1 || engine.raws[0] != `{"cmd":"kick","nick":"Alice"}` || status != model.SUCCESSFUL || err != nil {
		t.Fatalf("repeated selection replayed same moderation action: frames=%v status=%v err=%v", engine.raws, status, err)
	}
}

func TestModerationSelectorAuditKickValidModesResolveBeforeEffects(t *testing.T) {
	users := map[string]*model.User{
		"first":  {Name: "First"},
		"second": {Name: "Second"},
	}
	definition, _ := commandDefinitionFor("kick")
	for _, test := range []struct {
		name string
		text string
		want []string
	}{
		{name: "exact mention and canonical case", text: "!kick @FIRST", want: []string{`{"cmd":"kick","nick":"First"}`}},
		{name: "multi skips absent", text: "!kick -m missing @SECOND first", want: []string{`{"cmd":"kick","nick":"Second"}`, `{"cmd":"kick","nick":"First"}`}},
		{name: "contains", text: "!kick -c ir", want: []string{`{"cmd":"kick","nick":"First"}`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := &commandEngineStub{users: users}
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: test.text}).Execute(context.Background())
			if status != model.SUCCESSFUL || err != nil || !equalStrings(engine.raws, test.want) {
				t.Fatalf("status=%v err=%v frames=%v, want %v", status, err, engine.raws, test.want)
			}
		})
	}
}

func TestModerationSelectorAuditKickContainsPreservesSourceCanonicalNames(t *testing.T) {
	engine := &commandEngineStub{users: map[string]*model.User{
		"plain":   {Name: "alice"},
		"literal": {Name: "@alice"},
	}}
	definition, _ := commandDefinitionFor("kick")
	status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!kick -c alice"}).Execute(context.Background())
	if status != model.SUCCESSFUL || err != nil || !equalStrings(engine.raws, []string{
		`{"cmd":"kick","nick":"@alice"}`,
		`{"cmd":"kick","nick":"alice"}`,
	}) {
		t.Fatalf("status=%v err=%v frames=%v", status, err, engine.raws)
	}
}

func TestModerationSelectorAuditKickStopsAfterCancellationOrSendFailure(t *testing.T) {
	users := map[string]*model.User{"first": {Name: "First"}, "second": {Name: "Second"}}
	definition, _ := commandDefinitionFor("kick")
	t.Run("pre-execution cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		engine := &moderationSelectorKickEngine{commandEngineStub: &commandEngineStub{users: users}}
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!kick first"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) || len(engine.raws) != 0 {
			t.Fatalf("status=%v err=%v frames=%v", status, err, engine.raws)
		}
	})
	t.Run("mid-execution cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		engine := &moderationSelectorKickEngine{commandEngineStub: &commandEngineStub{users: users}, cancel: cancel}
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!kick -m first second"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) || len(engine.raws) != 1 {
			t.Fatalf("status=%v err=%v frames=%v", status, err, engine.raws)
		}
	})
	t.Run("later send failure", func(t *testing.T) {
		sendErr := errors.New("second send failed")
		engine := &moderationSelectorKickEngine{commandEngineStub: &commandEngineStub{users: users}, errors: []error{nil, sendErr}}
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!kick -m first second"}).Execute(context.Background())
		if status != model.FAILED || !errors.Is(err, sendErr) || len(engine.raws) != 2 {
			t.Fatalf("status=%v err=%v frames=%v", status, err, engine.raws)
		}
	})
}

func TestModerationSelectorAuditEmptyMatchIsNotSuccessfulAction(t *testing.T) {
	for _, canonical := range []string{"kick", "shadowban"} {
		t.Run(canonical, func(t *testing.T) {
			repo := &shadowBanRepositoryStub{}
			engine := newShadowBanEngine(repo, map[string]*model.User{"alice": {Name: "alice"}})
			definition, _ := commandDefinitionFor(canonical)
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!" + canonical + " -c absent"}).Execute(context.Background())
			if status == model.SUCCESSFUL || len(repo.persisted) != 0 || len(engine.kicked) != 0 {
				t.Fatalf("no matching user became successful action: status=%v err=%v writes=%v kicks=%v", status, err, repo.persisted, engine.kicked)
			}
		})
	}
}

func TestModerationSelectorAuditUnshadowUsesChangedRowsAndRetainsReceiptOnExit(t *testing.T) {
	definition, _ := commandDefinitionFor("unshadowban")
	t.Run("all reports actual delete count without listing", func(t *testing.T) {
		repo := &shadowBanRepositoryStub{records: []repository.ShadowBanRecord{{Name: "one"}, {Name: "two"}}, removeAllCount: 1}
		engine := newShadowBanEngine(repo, nil)
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!unshadowban -all"}).Execute(context.Background())
		if status != model.SUCCESSFUL || err != nil || repo.listCalls != 0 || !equalStrings(engine.chats, []string{"mod|Removed shadow-ban records: 1|false"}) {
			t.Fatalf("status=%v err=%v listCalls=%d chats=%v", status, err, repo.listCalls, engine.chats)
		}
	})
	for _, test := range []struct {
		name string
		text string
	}{
		{name: "single zero rows", text: "!unshadowban absent"},
		{name: "all zero rows", text: "!unshadowban -all"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &shadowBanRepositoryStub{}
			engine := newShadowBanEngine(repo, nil)
			status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: test.text}).Execute(context.Background())
			if status == model.SUCCESSFUL || err != nil || len(engine.chats) != 1 {
				t.Fatalf("status=%v err=%v chats=%v", status, err, engine.chats)
			}
		})
	}
	t.Run("cancellation after delete retains commit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		ctx, receipt := common.WithMutationRecorder(ctx)
		repo := &shadowBanRepositoryStub{removeCount: 2, afterRemove: cancel}
		engine := newShadowBanEngine(repo, nil)
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!unshadowban target"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, context.Canceled) || receipt.Count() != 1 || len(engine.chats) != 0 {
			t.Fatalf("status=%v err=%v receipt=%d chats=%v", status, err, receipt.Count(), engine.chats)
		}
	})
	t.Run("ack failure retains commit", func(t *testing.T) {
		ctx, receipt := common.WithMutationRecorder(context.Background())
		ackErr := errors.New("ack failed")
		repo := &shadowBanRepositoryStub{removeAllCount: 4}
		engine := newShadowBanEngine(repo, nil)
		engine.replyErr = ackErr
		status, err := definition.New(engine, &model.ChatMessage{Name: "mod", Text: "!unshadowban -all"}).Execute(ctx)
		if status != model.FAILED || !errors.Is(err, ackErr) || receipt.Count() != 1 || len(engine.chats) != 1 {
			t.Fatalf("status=%v err=%v receipt=%d chats=%v", status, err, receipt.Count(), engine.chats)
		}
	})
}
