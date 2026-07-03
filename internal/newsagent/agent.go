package newsagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/techbudol1/budol-api/internal/config"
	"github.com/techbudol1/budol-api/internal/store"

	"github.com/cloudwego/eino/compose"
)

type CandidateStore interface {
	CreateNewsCandidateIfMissing(ctx context.Context, input store.NewsCandidateInput) (store.NewsCandidate, bool, error)
}

type Fetcher interface {
	Fetch(ctx context.Context, query string, limit int) ([]Article, error)
}

type Drafter interface {
	Draft(ctx context.Context, group ArticleGroup) (PollDraft, error)
}

type Article struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Domain        string `json:"domain"`
	PublishedAt   string `json:"published_at"`
	SourceCountry string `json:"source_country"`
	IsPrimary     bool   `json:"is_primary"`
}

type ArticleGroup struct {
	Headline string
	Articles []Article
}

type PollDraft struct {
	Eligible              bool     `json:"eligible" jsonschema_description:"Whether this event can support a clear, objectively resolvable prediction market"`
	RejectionReason       string   `json:"rejection_reason" jsonschema_description:"Why the event is not eligible, empty when eligible"`
	ProposedTitle         string   `json:"proposed_title" jsonschema_description:"Neutral future-tense poll question ending in a question mark"`
	PollType              string   `json:"poll_type" jsonschema:"enum=yes_no,enum=head_to_head,enum=polling_leader,enum=threshold,enum=deadline,enum=bill_passage,enum=multiple_choice_yes_no,enum=approval_rating,enum=turnout,enum=court_decision,enum=appointment,enum=budget_release"`
	ClassificationID      string   `json:"classification_id" jsonschema:"enum=elections,enum=congress,enum=lgu,enum=policy"`
	CallName              string   `json:"call_name" jsonschema_description:"Short market type label"`
	Region                string   `json:"region" jsonschema_description:"Philippine geographic area or institution"`
	OutcomeA              string   `json:"outcome_a" jsonschema_description:"Concise affirmative or first outcome"`
	OutcomeB              string   `json:"outcome_b" jsonschema_description:"Concise negative or second outcome"`
	ChoiceLabels          []string `json:"choice_labels" jsonschema_description:"For multiple_choice_yes_no only: 2 to 8 distinct named choices explicitly supported by the source metadata; otherwise an empty array"`
	ResolutionSource      string   `json:"resolution_source" jsonschema_description:"Named primary authority or reputable source that can settle the poll"`
	ResolutionEvidenceURL string   `json:"resolution_evidence_url" jsonschema_description:"One supplied source URL most relevant to resolution"`
	ResolutionNotes       string   `json:"resolution_notes" jsonschema_description:"Exact objective resolution rule, deadline, timezone, and ambiguity handling"`
	StartsAt              string   `json:"starts_at" jsonschema_description:"RFC3339 start time or empty string"`
	EndsAt                string   `json:"ends_at" jsonschema_description:"RFC3339 trading deadline"`
	Confidence            int64    `json:"confidence" jsonschema_description:"Integer confidence from 0 to 100"`
	Rationale             string   `json:"rationale" jsonschema_description:"Short explanation grounded only in supplied headlines"`
	SafetyFlags           []string `json:"safety_flags" jsonschema_description:"Potential misinformation, sensitivity, ambiguity, or manipulation concerns"`
}

type scanInput struct {
	Query  string
	Limit  int
	Filter string
}

type fetchedBatch struct {
	Articles []Article
	Filter   string
}

type screenedBatch struct {
	Fetched int
	Groups  []ArticleGroup
}

type draftedBatch struct {
	Fetched  int
	Screened int
	Items    []draftedItem
}

type draftedItem struct {
	Group ArticleGroup
	Draft PollDraft
}

type ScanResult struct {
	Filter    string `json:"filter"`
	Fetched   int    `json:"fetched"`
	Screened  int    `json:"screened"`
	Drafted   int    `json:"drafted"`
	Created   int    `json:"created"`
	Duplicate int    `json:"duplicate"`
	Skipped   int    `json:"skipped"`
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
}

const (
	ScanFilterNoSkip                = "no_skip"
	ScanFilterAll                   = "all"
	ScanFilterCorroboratedOrPrimary = "corroborated_or_primary"
	ScanFilterCorroborated          = "corroborated"
	ScanFilterPrimary               = "primary"
)

type Status struct {
	Configured    bool       `json:"configured"`
	Enabled       bool       `json:"enabled"`
	Framework     string     `json:"framework"`
	Model         string     `json:"model"`
	Interval      string     `json:"interval"`
	Query         string     `json:"query"`
	AutoPublishes bool       `json:"autoPublishes"`
	Running       bool       `json:"running"`
	LastRun       ScanResult `json:"lastRun"`
}

type Agent struct {
	cfg        config.Config
	store      CandidateStore
	fetcher    Fetcher
	drafter    Drafter
	workflow   compose.Runnable[scanInput, draftedBatch]
	sourceSet  map[string]bool
	primarySet map[string]bool

	mu      sync.Mutex
	running bool
	lastRun ScanResult
}

func New(ctx context.Context, cfg config.Config, candidateStore CandidateStore) (*Agent, error) {
	agent := &Agent{
		cfg:        cfg,
		store:      candidateStore,
		sourceSet:  domainSet(cfg.NewsAgentSourceDomains),
		primarySet: domainSet(cfg.NewsAgentPrimaryDomains),
	}
	gdelt := NewGDELTFetcher(cfg.NewsAgentGDELTURL, agent.sourceSet, agent.primarySet)
	rss := NewRSSFetcher(cfg.NewsAgentRSSURLs, agent.sourceSet, agent.primarySet)
	agent.fetcher = NewCachedFetcher(NewFallbackFetcher(gdelt, rss), 5*time.Minute)
	if strings.TrimSpace(cfg.OpenAIAPIKey) != "" {
		agent.drafter = NewOpenAIDrafter(cfg.OpenAIAPIKey, cfg.OpenAIModel)
	}
	if !agent.Configured() {
		return agent, nil
	}

	graph := compose.NewGraph[scanInput, draftedBatch]()
	if err := graph.AddLambdaNode("retrieve", compose.InvokableLambda(agent.retrieve)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("screen", compose.InvokableLambda(agent.screen)); err != nil {
		return nil, err
	}
	if err := graph.AddLambdaNode("draft", compose.InvokableLambda(agent.draft)); err != nil {
		return nil, err
	}
	if err := graph.AddEdge(compose.START, "retrieve"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("retrieve", "screen"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("screen", "draft"); err != nil {
		return nil, err
	}
	if err := graph.AddEdge("draft", compose.END); err != nil {
		return nil, err
	}
	workflow, err := graph.Compile(ctx, compose.WithGraphName("budol-news-scout"))
	if err != nil {
		return nil, err
	}
	agent.workflow = workflow
	return agent, nil
}

func (a *Agent) Configured() bool {
	return a != nil && a.cfg.NewsAgentEnabled && a.store != nil && a.fetcher != nil && a.drafter != nil
}

func (a *Agent) Status() Status {
	if a == nil {
		return Status{Framework: "CloudWeGo Eino", AutoPublishes: false}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return Status{
		Configured:    a.Configured(),
		Enabled:       a.cfg.NewsAgentEnabled,
		Framework:     "CloudWeGo Eino",
		Model:         a.cfg.OpenAIModel,
		Interval:      a.cfg.NewsAgentInterval.String(),
		Query:         a.cfg.NewsAgentQuery,
		AutoPublishes: false,
		Running:       a.running,
		LastRun:       a.lastRun,
	}
}

func (a *Agent) Start(ctx context.Context) {
	if !a.Configured() {
		return
	}
	interval := a.cfg.NewsAgentInterval
	if interval < time.Minute {
		interval = 15 * time.Minute
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		_, _ = a.Scan(ctx)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = a.Scan(ctx)
		}
	}
}

func (a *Agent) Scan(ctx context.Context) (result ScanResult, scanErr error) {
	return a.ScanWithFilter(ctx, ScanFilterNoSkip)
}

func (a *Agent) ScanWithFilter(ctx context.Context, filter string) (result ScanResult, scanErr error) {
	if !a.Configured() {
		return ScanResult{}, errors.New("news agent requires NEWS_AGENT_ENABLED=true and OPENAI_API_KEY")
	}
	if !ValidScanFilter(filter) {
		return ScanResult{}, fmt.Errorf("unsupported news scan filter %q", filter)
	}
	filter = normalizeScanFilter(filter)
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return ScanResult{}, errors.New("news agent scan is already running")
	}
	a.running = true
	a.mu.Unlock()

	result = ScanResult{Filter: filter, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	defer func() {
		result.EndedAt = time.Now().UTC().Format(time.RFC3339)
		a.mu.Lock()
		a.running = false
		a.lastRun = result
		a.mu.Unlock()
	}()

	batch, err := a.workflow.Invoke(ctx, scanInput{Query: a.cfg.NewsAgentQuery, Limit: a.cfg.NewsAgentMaxArticles, Filter: filter})
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.Fetched = batch.Fetched
	result.Screened = batch.Screened
	for _, item := range batch.Items {
		result.Drafted++
		item.Draft = retainForHumanReview(item.Draft, item.Group)
		input := candidateInput(item.Group, item.Draft, a.cfg.OpenAIModel)
		_, created, err := a.store.CreateNewsCandidateIfMissing(ctx, input)
		if err != nil {
			result.Error = err.Error()
			return result, err
		}
		if created {
			result.Created++
		} else {
			result.Duplicate++
		}
	}
	return result, nil
}

func retainForHumanReview(draft PollDraft, group ArticleGroup) PollDraft {
	if strings.TrimSpace(draft.ProposedTitle) == "" {
		draft.ProposedTitle = strings.TrimSpace(group.Headline)
	}
	if strings.TrimSpace(draft.Rationale) == "" {
		draft.Rationale = defaultText(draft.RejectionReason, "Single-source news lead retained for administrator verification.")
	}
	if !draft.Eligible {
		reason := defaultText(draft.RejectionReason, "AI did not recommend this story as a ready-to-use poll.")
		draft.SafetyFlags = appendUnique(draft.SafetyFlags, "Human verification required: "+reason)
	}
	if draft.Confidence < 60 {
		draft.SafetyFlags = appendUnique(draft.SafetyFlags, "Low-confidence AI draft; verify the source and redesign the poll if needed.")
	}
	return draft
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), value) {
			return values
		}
	}
	return append(values, value)
}

func (a *Agent) retrieve(ctx context.Context, input scanInput) (fetchedBatch, error) {
	articles, err := a.fetcher.Fetch(ctx, input.Query, input.Limit)
	if err != nil {
		return fetchedBatch{}, err
	}
	return fetchedBatch{Articles: articles, Filter: input.Filter}, nil
}

func (a *Agent) screen(_ context.Context, input fetchedBatch) (screenedBatch, error) {
	groups := groupArticles(input.Articles, a.cfg.NewsAgentMaxCandidates, input.Filter)
	return screenedBatch{Fetched: len(input.Articles), Groups: groups}, nil
}

func (a *Agent) draft(ctx context.Context, input screenedBatch) (draftedBatch, error) {
	items := make([]draftedItem, 0, len(input.Groups))
	for _, group := range input.Groups {
		draft, err := a.drafter.Draft(ctx, group)
		if err != nil {
			return draftedBatch{}, err
		}
		draft = normalizeDraft(draft, group)
		items = append(items, draftedItem{Group: group, Draft: draft})
	}
	return draftedBatch{Fetched: input.Fetched, Screened: len(input.Groups), Items: items}, nil
}

func groupArticles(articles []Article, limit int, filter string) []ArticleGroup {
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	groups := []ArticleGroup{}
	for _, article := range articles {
		tokens := titleTokens(article.Title)
		if len(tokens) < 3 {
			continue
		}
		bestIndex := -1
		bestScore := 0.0
		for i := range groups {
			score := jaccard(tokens, titleTokens(groups[i].Headline))
			if score > bestScore {
				bestScore = score
				bestIndex = i
			}
		}
		if bestIndex >= 0 && bestScore >= 0.28 {
			if !groupHasDomain(groups[bestIndex], article.Domain) {
				groups[bestIndex].Articles = append(groups[bestIndex].Articles, article)
			}
			continue
		}
		groups = append(groups, ArticleGroup{Headline: article.Title, Articles: []Article{article}})
	}
	eligible := make([]ArticleGroup, 0, len(groups))
	for _, group := range groups {
		if groupMatchesFilter(group, filter) {
			eligible = append(eligible, group)
		}
		if len(eligible) >= limit {
			break
		}
	}
	return eligible
}

func ValidScanFilter(filter string) bool {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", ScanFilterNoSkip, ScanFilterAll, ScanFilterCorroboratedOrPrimary, ScanFilterCorroborated, ScanFilterPrimary:
		return true
	default:
		return false
	}
}

func normalizeScanFilter(filter string) string {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return ScanFilterNoSkip
	}
	return filter
}

func groupMatchesFilter(group ArticleGroup, filter string) bool {
	switch normalizeScanFilter(filter) {
	case ScanFilterCorroboratedOrPrimary:
		return len(group.Articles) >= 2 || groupHasPrimary(group)
	case ScanFilterCorroborated:
		return len(group.Articles) >= 2
	case ScanFilterPrimary:
		return groupHasPrimary(group)
	default:
		return true
	}
}

func candidateInput(group ArticleGroup, draft PollDraft, model string) store.NewsCandidateInput {
	sources := make([]store.NewsSource, 0, len(group.Articles))
	for _, article := range group.Articles {
		sources = append(sources, store.NewsSource{
			Title:         article.Title,
			URL:           article.URL,
			Domain:        article.Domain,
			PublishedAt:   article.PublishedAt,
			SourceCountry: article.SourceCountry,
			IsPrimary:     article.IsPrimary,
		})
	}
	return store.NewsCandidateInput{
		Fingerprint:           candidateFingerprint(group),
		Headline:              group.Headline,
		ProposedTitle:         draft.ProposedTitle,
		PollType:              draft.PollType,
		ClassificationID:      draft.ClassificationID,
		CallName:              draft.CallName,
		Region:                draft.Region,
		OutcomeA:              draft.OutcomeA,
		OutcomeB:              draft.OutcomeB,
		ChoiceLabels:          draft.ChoiceLabels,
		ResolutionSource:      draft.ResolutionSource,
		ResolutionEvidenceURL: draft.ResolutionEvidenceURL,
		ResolutionNotes:       draft.ResolutionNotes,
		StartsAt:              draft.StartsAt,
		EndsAt:                draft.EndsAt,
		Confidence:            draft.Confidence,
		Rationale:             draft.Rationale,
		SafetyFlags:           draft.SafetyFlags,
		Sources:               sources,
		Model:                 model,
	}
}

func normalizeDraft(draft PollDraft, group ArticleGroup) PollDraft {
	draft.ProposedTitle = strings.TrimSpace(draft.ProposedTitle)
	if draft.ProposedTitle != "" && !strings.HasSuffix(draft.ProposedTitle, "?") {
		draft.ProposedTitle += "?"
	}
	if !validPollType(draft.PollType) {
		draft.PollType = "yes_no"
	}
	if !validClassification(draft.ClassificationID) {
		draft.ClassificationID = "policy"
	}
	draft.CallName = defaultText(draft.CallName, "news market")
	draft.Region = defaultText(draft.Region, "National")
	draft.OutcomeA = defaultText(draft.OutcomeA, "Yes")
	draft.OutcomeB = defaultText(draft.OutcomeB, "No")
	draft.ChoiceLabels = normalizedChoiceLabels(draft.ChoiceLabels)
	if draft.PollType == "multiple_choice_yes_no" {
		if len(draft.ChoiceLabels) < 2 {
			draft.PollType = "yes_no"
			draft.ChoiceLabels = nil
			draft.SafetyFlags = appendUnique(draft.SafetyFlags, "Multiple-choice suggestion lacked at least two distinct source-supported choices; converted to yes/no.")
		} else {
			draft.OutcomeA = "Yes"
			draft.OutcomeB = "No"
		}
	} else {
		draft.ChoiceLabels = nil
	}
	if draft.Confidence < 0 {
		draft.Confidence = 0
	}
	if draft.Confidence > 100 {
		draft.Confidence = 100
	}
	if !groupHasURL(group, draft.ResolutionEvidenceURL) && len(group.Articles) > 0 {
		draft.ResolutionEvidenceURL = group.Articles[0].URL
	}
	if draft.EndsAt != "" {
		if deadline, err := time.Parse(time.RFC3339, draft.EndsAt); err != nil || deadline.Before(time.Now().UTC()) || deadline.After(time.Now().UTC().AddDate(1, 0, 0)) {
			draft.Eligible = false
			draft.RejectionReason = "invalid or unsafe trading deadline"
		}
	}
	if draft.ProposedTitle == "" || draft.ResolutionNotes == "" || draft.EndsAt == "" {
		draft.Eligible = false
		draft.RejectionReason = "draft is missing a question, deadline, or objective resolution rule"
	}
	return draft
}

func candidateFingerprint(group ArticleGroup) string {
	parts := make([]string, 0, len(group.Articles)+1)
	parts = append(parts, strings.Join(sortedTokenKeys(titleTokens(group.Headline)), " "))
	for _, article := range group.Articles {
		parts = append(parts, strings.ToLower(strings.TrimSpace(article.URL)))
	}
	sort.Strings(parts[1:])
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func domainSet(domains []string) map[string]bool {
	result := map[string]bool{}
	for _, domain := range domains {
		domain = normalizeDomain(domain)
		if domain != "" {
			result[domain] = true
		}
	}
	return result
}

func domainAllowed(domain string, domains map[string]bool) bool {
	domain = normalizeDomain(domain)
	for allowed := range domains {
		if domain == allowed || strings.HasSuffix(domain, "."+allowed) {
			return true
		}
	}
	return false
}

func normalizeDomain(domain string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")
}

func groupHasDomain(group ArticleGroup, domain string) bool {
	for _, article := range group.Articles {
		if article.Domain == domain {
			return true
		}
	}
	return false
}

func groupHasPrimary(group ArticleGroup) bool {
	for _, article := range group.Articles {
		if article.IsPrimary {
			return true
		}
	}
	return false
}

func groupHasURL(group ArticleGroup, rawURL string) bool {
	for _, article := range group.Articles {
		if strings.EqualFold(strings.TrimSpace(article.URL), strings.TrimSpace(rawURL)) {
			return true
		}
	}
	return false
}

func titleTokens(title string) map[string]bool {
	replacer := strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "?", " ", "!", " ", "\"", " ", "'", " ", "(", " ", ")", " ", "-", " ")
	stop := map[string]bool{"the": true, "a": true, "an": true, "to": true, "of": true, "in": true, "on": true, "for": true, "and": true, "or": true, "with": true, "at": true, "from": true, "as": true, "is": true, "are": true, "will": true, "says": true, "said": true, "philippines": true, "philippine": true}
	result := map[string]bool{}
	for _, token := range strings.Fields(strings.ToLower(replacer.Replace(title))) {
		if len(token) >= 3 && !stop[token] {
			result[token] = true
		}
	}
	return result
}

func jaccard(left map[string]bool, right map[string]bool) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	union := map[string]bool{}
	for token := range left {
		union[token] = true
		if right[token] {
			intersection++
		}
	}
	for token := range right {
		union[token] = true
	}
	return float64(intersection) / float64(len(union))
}

func sortedTokenKeys(tokens map[string]bool) []string {
	keys := make([]string, 0, len(tokens))
	for token := range tokens {
		keys = append(keys, token)
	}
	sort.Strings(keys)
	return keys
}

func validPollType(value string) bool {
	switch value {
	case "yes_no", "head_to_head", "polling_leader", "threshold", "deadline", "bill_passage", "multiple_choice_yes_no", "approval_rating", "turnout", "court_decision", "appointment", "budget_release":
		return true
	default:
		return false
	}
}

func normalizedChoiceLabels(values []string) []string {
	choices := make([]string, 0, min(len(values), 8))
	seen := map[string]bool{}
	for _, value := range values {
		choice := strings.TrimSpace(value)
		key := strings.ToLower(choice)
		if choice == "" || seen[key] {
			continue
		}
		seen[key] = true
		choices = append(choices, choice)
		if len(choices) == 8 {
			break
		}
	}
	return choices
}

func validClassification(value string) bool {
	switch value {
	case "elections", "congress", "lgu", "policy":
		return true
	default:
		return false
	}
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func (r ScanResult) String() string {
	return fmt.Sprintf("fetched=%d screened=%d drafted=%d created=%d duplicate=%d skipped=%d", r.Fetched, r.Screened, r.Drafted, r.Created, r.Duplicate, r.Skipped)
}
