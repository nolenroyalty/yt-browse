package tui

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/nroyalty/yt-browse/internal/cache"
	"github.com/nroyalty/yt-browse/internal/config"
	"github.com/nroyalty/yt-browse/internal/recent"
	"github.com/nroyalty/yt-browse/internal/youtube"
	"github.com/sahilm/fuzzy"
)

type viewMode int

const (
	viewPlaylists viewMode = iota
	viewVideos
	viewPlaylistVideos // drill-in: viewing videos inside a specific playlist
)

type loadState int

const (
	loadIdle loadState = iota
	loadLoading
	loadDone
	loadError
)

type sortField int

const (
	sortNone sortField = iota
	sortByDate
	sortByViews
	sortByDuration
	sortByCount
)

type sortDir int

const (
	sortDesc sortDir = iota // natural: newest, most views, longest
	sortAsc                 // reverse: oldest, fewest views, shortest
)

type filterMode int

const (
	filterFuzzy filterMode = iota
	filterExact
	filterWords
	filterRegex
)

var filterModes = []filterMode{filterFuzzy, filterExact, filterWords, filterRegex}

func (fm filterMode) String() string {
	switch fm {
	case filterFuzzy:
		return "fuzzy"
	case filterExact:
		return "exact"
	case filterWords:
		return "words"
	case filterRegex:
		return "regex"
	}
	return "fuzzy"
}

type Model struct {
	cfg      *config.Config
	ytClient *youtube.Client
	cache    *cache.Store
	keys     keyMap

	// Recent channels
	recentStore     *recent.Store
	pickerMode      bool
	pickerResolving bool
	pickerList      list.Model
	pickerInput     textinput.Model

	// Channel
	channelInput string
	channel      *youtube.Channel

	// View state
	activeView viewMode
	width      int
	height     int

	// Playlists
	playlistList      list.Model
	playlistLoadState loadState
	playlists         []youtube.Playlist
	playlistSort      sortField
	playlistSortDir   sortDir

	// Videos (uploads)
	videoList       list.Model
	videoLoadState  loadState
	videos          []youtube.Video
	videoSort       sortField
	videoSortDir    sortDir
	videoProgressCh <-chan videoLoadingMsg
	videoTotal      int
	videoLoaded     int
	videoFetchGen   int
	videoCancel     context.CancelFunc

	// Playlist videos (drill-in)
	currentPlaylist        *youtube.Playlist
	playlistVideoList      list.Model
	playlistVideoLoadState loadState
	playlistVideos         []youtube.Video
	playlistVideoSort      sortField
	playlistVideoSortDir   sortDir

	// Filter: we manage our own filter instead of using the list's built-in one
	filterInput        textinput.Model
	filtering          bool         // is the filter input active/focused
	filterText         string       // the applied filter text (persists after closing input)
	filterMode         filterMode   // fuzzy or exact
	fstate             *filterState // shared with delegate for match highlighting
	sortOverridesFuzzy bool         // user manually changed sort while fuzzy filter is active
	filterTitlesOnly   bool         // only search titles (not descriptions) in non-fuzzy modes

	// Detail
	detailViewport viewport.Model
	showDetail     bool

	// Help overlay
	showHelp bool

	// Status flash (e.g. "Copied!")
	flashMessage string
	flashExpiry  time.Time

	// Error
	lastError error
}

func New(cfg *config.Config, ytClient *youtube.Client, cacheStore *cache.Store, recentStore *recent.Store, channelInput string) Model {
	fs := &filterState{flashIndex: -1}
	delegate := newHighlightDelegate(fs)

	playlistList := list.New([]list.Item{}, delegate, 0, 0)
	playlistList.SetShowTitle(false)
	playlistList.SetShowHelp(false)
	playlistList.SetShowStatusBar(true)
	playlistList.SetFilteringEnabled(false) // we handle filtering ourselves
	playlistList.DisableQuitKeybindings()

	videoList := list.New([]list.Item{}, delegate, 0, 0)
	videoList.SetShowTitle(false)
	videoList.SetShowHelp(false)
	videoList.SetShowStatusBar(true)
	videoList.SetFilteringEnabled(false)
	videoList.DisableQuitKeybindings()

	playlistVideoList := list.New([]list.Item{}, delegate, 0, 0)
	playlistVideoList.SetShowTitle(false)
	playlistVideoList.SetShowHelp(false)
	playlistVideoList.SetShowStatusBar(true)
	playlistVideoList.SetFilteringEnabled(false)
	playlistVideoList.DisableQuitKeybindings()

	pickerList := list.New([]list.Item{}, delegate, 0, 0)
	pickerList.SetShowTitle(false)
	pickerList.SetShowHelp(false)
	pickerList.SetShowStatusBar(true)
	pickerList.SetFilteringEnabled(false)
	pickerList.DisableQuitKeybindings()

	fi := textinput.New()
	fi.Prompt = "  / "
	styles := fi.Styles()
	styles.Focused.Prompt = filterPromptStyle
	styles.Focused.Text = filterTextStyle
	fi.SetStyles(styles)

	pi := textinput.New()
	pi.Prompt = "> "
	pi.Placeholder = "Filter or enter new channel"
	piStyles := pi.Styles()
	piStyles.Focused.Prompt = filterPromptStyle
	piStyles.Focused.Text = filterTextStyle
	pi.SetStyles(piStyles)
	if channelInput == "" {
		pi.Focus()
	}

	vp := viewport.New()

	return Model{
		cfg:               cfg,
		ytClient:          ytClient,
		cache:             cacheStore,
		keys:              defaultKeyMap(),
		recentStore:       recentStore,
		pickerMode:        channelInput == "",
		pickerList:        pickerList,
		pickerInput:       pi,
		channelInput:      channelInput,
		activeView:        viewVideos,
		playlistList:      playlistList,
		videoList:         videoList,
		playlistVideoList: playlistVideoList,
		filterInput:       fi,
		filterMode:        filterFuzzy,
		fstate:            fs,
		detailViewport:    vp,
		showDetail:        true,
		playlistSort:      sortNone,
		videoSort:         sortByDate,
		videoSortDir:      sortDesc,
	}
}

func (m Model) Init() tea.Cmd {
	if m.pickerMode {
		// Focus() was already called in New() to set internal state.
		// Call it again here just to get the cursor blink cmd (Init has a
		// value receiver so the state change on this copy is harmless).
		return tea.Batch(m.pickerInput.Focus(), loadRecentCmd(m.recentStore))
	}
	return resolveChannelCmd(m.ytClient, m.cache, m.channelInput)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()
		if !m.pickerMode {
			m.updateDetail()
		}
		return m, nil

	case recentChannelsLoadedMsg:
		items := make([]list.Item, len(msg.entries))
		for i, e := range msg.entries {
			items[i] = RecentItem{entry: e}
		}
		m.pickerList.SetItems(items)
		m.updateSizes()
		return m, nil

	case tea.KeyPressMsg:
		if m.pickerMode {
			return m.handlePickerKey(msg)
		}
		// Dismiss help overlay on any key
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		// When filter input is active, handle keys for the filter
		if m.filtering {
			return m.handleFilterKey(msg)
		}

		switch {
		case key.Matches(msg, m.keys.Help):
			m.showHelp = true
			return m, nil

		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Filter):
			m.filtering = true
			m.filterInput.SetValue(m.filterText)
			m.filterInput.Focus()
			m.filterInput.CursorEnd()
			m.updateSizes() // filter bar appeared
			return m, nil

		case key.Matches(msg, m.keys.ClearFilter):
			// In drill view with no active filter, backspace/esc goes back
			if m.filterText == "" && m.activeView == viewPlaylistVideos {
				return m.handleBack()
			}
			if m.filterText != "" {
				m.filterText = ""
				m.sortOverridesFuzzy = false
				m.applyFilterAndSort()
				m.updateSizes() // filter bar disappeared
			}
			return m, nil

		case key.Matches(msg, m.keys.Back):
			if m.activeView == viewPlaylistVideos {
				return m.handleBack()
			}
			return m, nil

		case key.Matches(msg, m.keys.OpenURL):
			return m.handleOpenURL()

		case key.Matches(msg, m.keys.CopyURL):
			return m.handleCopyURL()

		case key.Matches(msg, m.keys.OpenPlaylist):
			if m.activeView == viewPlaylistVideos && m.currentPlaylist != nil {
				return m, openURLCmd(m.currentPlaylist.URL())
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleFilterMode):
			// Cycle through filter modes
			for i, fm := range filterModes {
				if fm == m.filterMode {
					m.filterMode = filterModes[(i+1)%len(filterModes)]
					break
				}
			}
			if m.filterText != "" {
				m.applyFilterAndSort()
				m.updateDetail()
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleFilterScope):
			m.filterTitlesOnly = !m.filterTitlesOnly
			if m.filterText != "" {
				m.applyFilterAndSort()
				m.updateDetail()
			}
			return m, nil

		case key.Matches(msg, m.keys.Tab):
			return m.handleTabSwitch()

		case key.Matches(msg, m.keys.Enter):
			return m.handleEnter()

		case key.Matches(msg, m.keys.SortDate):
			m.toggleSort(sortByDate, sortDesc)
			m.applyFilterAndSort()
			m.updateDetail()
			return m, nil

		case key.Matches(msg, m.keys.SortDateRev):
			m.toggleSort(sortByDate, sortAsc)
			m.applyFilterAndSort()
			m.updateDetail()
			return m, nil

		case key.Matches(msg, m.keys.SortViews):
			if m.activeView == viewVideos || m.activeView == viewPlaylistVideos {
				m.toggleSort(sortByViews, sortDesc)
				m.applyFilterAndSort()
				m.updateDetail()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.SortViewsRev):
			if m.activeView == viewVideos || m.activeView == viewPlaylistVideos {
				m.toggleSort(sortByViews, sortAsc)
				m.applyFilterAndSort()
				m.updateDetail()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.SortDuration):
			if m.activeView == viewVideos || m.activeView == viewPlaylistVideos {
				m.toggleSort(sortByDuration, sortDesc)
				m.applyFilterAndSort()
				m.updateDetail()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.SortDurationRev):
			if m.activeView == viewVideos || m.activeView == viewPlaylistVideos {
				m.toggleSort(sortByDuration, sortAsc)
				m.applyFilterAndSort()
				m.updateDetail()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.SortCount):
			if m.activeView == viewPlaylists {
				m.toggleSort(sortByCount, sortDesc)
				m.applyFilterAndSort()
				m.updateDetail()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.SortCountRev):
			if m.activeView == viewPlaylists {
				m.toggleSort(sortByCount, sortAsc)
				m.applyFilterAndSort()
				m.updateDetail()
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.Refresh):
			return m.handleRefresh()

		}

	case channelResolvedMsg:
		m.channel = msg.channel
		m.pickerMode = false
		m.playlistLoadState = loadLoading
		m.updateSizes()
		return m, tea.Batch(
			fetchPlaylistsCmd(m.ytClient, m.cache, m.channel.ID, false),
			saveRecentCmd(m.recentStore, m.channel),
		)

	case channelErrorMsg:
		m.lastError = msg.err
		if m.pickerMode {
			m.pickerResolving = false
		}
		return m, nil

	case playlistsFetchedMsg:
		filtered := msg.playlists[:0]
		for _, p := range msg.playlists {
			if p.ItemCount > 0 {
				filtered = append(filtered, p)
			}
		}
		m.playlists = filtered
		m.playlistLoadState = loadDone
		m.applyFilterAndSort()
		m.updateDetail()
		// Start fetching videos in background
		if m.channel != nil && m.channel.LongFormPlaylistID != "" {
			m.videoLoadState = loadLoading
			return m, m.startVideoFetch(m.channel.LongFormPlaylistID, m.channel.ID, false)
		}
		return m, nil

	case playlistsErrorMsg:
		m.playlistLoadState = loadError
		m.lastError = msg.err
		return m, nil

	case videoLoadingMsg:
		if msg.gen != m.videoFetchGen {
			return m, nil
		}
		m.videoTotal = msg.total
		m.videoLoaded = msg.loaded
		return m, listenForVideoProgress(m.videoProgressCh)

	case videosFetchedMsg:
		if msg.gen != m.videoFetchGen {
			return m, nil
		}
		m.videos = msg.videos
		m.videoLoadState = loadDone
		m.clearVideoProgress()
		// Always populate video list so tab bar count is correct even if
		// we're on the playlists tab when this arrives in the background.
		m.videoList.SetItems(m.sortedVideoItems(m.videos, m.videoSort, m.videoSortDir))
		m.applyFilterAndSort()
		m.updateDetail()
		return m, nil

	case videosErrorMsg:
		if msg.gen != m.videoFetchGen {
			return m, nil
		}
		m.videoLoadState = loadError
		m.lastError = msg.err
		m.clearVideoProgress()
		return m, nil

	case playlistVideosFetchedMsg:
		m.playlistVideos = msg.videos
		m.playlistVideoLoadState = loadDone
		m.applyFilterAndSort()
		m.updateDetail()
		return m, nil

	case playlistVideosErrorMsg:
		m.playlistVideoLoadState = loadError
		m.lastError = msg.err
		return m, nil

	case clearFlashMsg:
		m.flashMessage = ""
		m.fstate.flashOn = false
		return m, nil

	}

	// Delegate to active list or picker input
	var cmd tea.Cmd
	if m.pickerMode {
		m.pickerInput, cmd = m.pickerInput.Update(msg)
	} else {
		switch m.activeView {
		case viewPlaylists:
			m.playlistList, cmd = m.playlistList.Update(msg)
		case viewVideos:
			m.videoList, cmd = m.videoList.Update(msg)
		case viewPlaylistVideos:
			m.playlistVideoList, cmd = m.playlistVideoList.Update(msg)
		}
		// Update detail pane
		m.updateDetail()
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	if m.pickerMode {
		return m.renderPickerView()
	}

	var sections []string

	sections = append(sections, m.renderHeader())
	sections = append(sections, m.renderTabBar())

	sections = append(sections, m.renderFilterBar())

	sections = append(sections, m.renderContent())
	sections = append(sections, m.renderHelpBar())

	var str string
	if m.showHelp {
		str = m.renderHelpOverlay()
	} else {
		str = lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	v := tea.NewView(str)
	v.AltScreen = true
	return v
}

// --- picker handling ---

func (m *Model) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	// Block input while resolving
	if m.pickerResolving {
		return m, nil
	}
	switch msg.String() {
	case "enter":
		// If the list has a selection, pick it
		if selected := m.pickerList.SelectedItem(); selected != nil {
			ri := selected.(RecentItem)
			m.channelInput = ri.entry.Handle
			if m.channelInput == "" {
				m.channelInput = ri.entry.ID
			}
			return m.resolvePickerChannel()
		}
		// No list results — treat input as a new channel lookup
		if input := strings.TrimSpace(m.pickerInput.Value()); input != "" {
			m.channelInput = input
			return m.resolvePickerChannel()
		}
		return m, nil
	case "ctrl+o":
		// Force lookup: treat input text as a new channel even if list has matches
		if input := strings.TrimSpace(m.pickerInput.Value()); input != "" {
			m.channelInput = input
			return m.resolvePickerChannel()
		}
		return m, nil
	case "up", "shift+tab", "ctrl+k", "ctrl+p":
		m.pickerList.CursorUp()
		return m, nil
	case "down", "tab", "ctrl+j", "ctrl+n":
		m.pickerList.CursorDown()
		return m, nil
	default:
		var cmd tea.Cmd
		m.pickerInput, cmd = m.pickerInput.Update(msg)
		// Live-filter the recent list as user types
		m.filterPickerList()
		return m, cmd
	}
}

func (m *Model) resolvePickerChannel() (tea.Model, tea.Cmd) {
	m.pickerResolving = true
	m.lastError = nil
	return m, resolveChannelCmd(m.ytClient, m.cache, m.channelInput)
}

func (m *Model) filterPickerList() {
	query := strings.TrimSpace(m.pickerInput.Value())
	if query == "" {
		// Reload all entries
		entries := m.recentStore.Load()
		items := make([]list.Item, len(entries))
		for i, e := range entries {
			items[i] = RecentItem{entry: e}
		}
		m.pickerList.SetItems(items)
		return
	}
	// Fuzzy filter
	entries := m.recentStore.Load()
	targets := make([]string, len(entries))
	for i, e := range entries {
		targets[i] = e.Handle + " " + e.Title
	}
	matches := fuzzy.Find(query, targets)
	items := make([]list.Item, len(matches))
	for i, match := range matches {
		items[i] = RecentItem{entry: entries[match.Index]}
	}
	m.pickerList.SetItems(items)
}

// --- filter handling ---

func (m *Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.String() == "enter":
		// Apply filter and close input
		m.filterText = m.filterInput.Value()
		m.filtering = false
		m.filterInput.Blur()
		m.applyFilterAndSort()
		m.updateDetail()
		m.updateSizes() // filter bar may have changed visibility
		return m, nil
	case msg.String() == "esc", msg.String() == "ctrl+c":
		// Cancel: revert to previous filter text
		m.filtering = false
		m.filterInput.Blur()
		m.filterInput.SetValue(m.filterText)
		m.updateSizes()
		return m, nil
	case msg.String() == "backspace" && m.filterInput.Value() == "":
		// Empty filter + backspace: exit filter mode
		m.filterText = ""
		m.filtering = false
		m.filterInput.Blur()
		m.sortOverridesFuzzy = false
		m.applyFilterAndSort()
		m.updateDetail()
		m.updateSizes()
		return m, nil
	case key.Matches(msg, m.keys.ToggleFilterMode):
		for i, fm := range filterModes {
			if fm == m.filterMode {
				m.filterMode = filterModes[(i+1)%len(filterModes)]
				break
			}
		}
		m.filterText = m.filterInput.Value()
		m.applyFilterAndSort()
		m.updateDetail()
		return m, nil
	case key.Matches(msg, m.keys.ToggleFilterScope):
		m.filterTitlesOnly = !m.filterTitlesOnly
		m.filterText = m.filterInput.Value()
		m.applyFilterAndSort()
		m.updateDetail()
		return m, nil
	default:
		// Forward to textinput
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		// Live-filter as user types
		m.filterText = m.filterInput.Value()
		m.applyFilterAndSort()
		m.updateDetail()
		return m, cmd
	}
}

// applyFilterAndSort is the core pipeline: raw data → sort → filter → SetItems
func (m *Model) applyFilterAndSort() {
	// Sync filter state to the delegate for match highlighting.
	// Strip date keywords so they don't produce garbage highlights.
	cleanText, _, _ := extractDateFilters(m.filterText)
	m.fstate.text = cleanText
	m.fstate.mode = m.filterMode

	// In fuzzy mode, relevance ranking wins over explicit sort — unless
	// the user manually toggled a sort while the fuzzy filter was active.
	useFuzzyRanking := m.filterText != "" && m.filterMode == filterFuzzy && !m.sortOverridesFuzzy

	switch m.activeView {
	case viewPlaylists:
		items := m.sortedPlaylistItems()
		if m.filterText != "" {
			items = m.filterItems(items, !useFuzzyRanking)
		}
		m.playlistList.SetItems(items)
	case viewVideos:
		items := m.sortedVideoItems(m.videos, m.videoSort, m.videoSortDir)
		if m.filterText != "" {
			items = m.filterItems(items, !useFuzzyRanking)
		}
		m.videoList.SetItems(items)
	case viewPlaylistVideos:
		items := m.sortedVideoItems(m.playlistVideos, m.playlistVideoSort, m.playlistVideoSortDir)
		if m.filterText != "" {
			items = m.filterItems(items, !useFuzzyRanking)
		}
		m.playlistVideoList.SetItems(items)
	}
}

func (m *Model) sortedPlaylistItems() []list.Item {
	sorted := make([]youtube.Playlist, len(m.playlists))
	copy(sorted, m.playlists)

	asc := m.playlistSortDir == sortAsc
	switch m.playlistSort {
	case sortByDate:
		sort.Slice(sorted, func(i, j int) bool {
			if asc {
				return sorted[i].PublishedAt.Before(sorted[j].PublishedAt)
			}
			return sorted[i].PublishedAt.After(sorted[j].PublishedAt)
		})
	case sortByCount:
		sort.Slice(sorted, func(i, j int) bool {
			if asc {
				return sorted[i].ItemCount < sorted[j].ItemCount
			}
			return sorted[i].ItemCount > sorted[j].ItemCount
		})
	}

	items := make([]list.Item, len(sorted))
	for i, p := range sorted {
		items[i] = PlaylistItem{playlist: p}
	}
	return items
}

func (m *Model) sortedVideoItems(videos []youtube.Video, sortBy sortField, dir sortDir) []list.Item {
	sorted := make([]youtube.Video, len(videos))
	copy(sorted, videos)

	asc := dir == sortAsc
	switch sortBy {
	case sortByDate:
		sort.Slice(sorted, func(i, j int) bool {
			if asc {
				return sorted[i].PublishedAt.Before(sorted[j].PublishedAt)
			}
			return sorted[i].PublishedAt.After(sorted[j].PublishedAt)
		})
	case sortByViews:
		sort.Slice(sorted, func(i, j int) bool {
			if asc {
				return sorted[i].ViewCount < sorted[j].ViewCount
			}
			return sorted[i].ViewCount > sorted[j].ViewCount
		})
	case sortByDuration:
		sort.Slice(sorted, func(i, j int) bool {
			if asc {
				return sorted[i].Duration < sorted[j].Duration
			}
			return sorted[i].Duration > sorted[j].Duration
		})
	}

	items := make([]list.Item, len(sorted))
	for i, v := range sorted {
		items[i] = VideoItem{video: v}
	}
	return items
}

// extractDateFilters parses before:/after: tokens from the query string,
// returning the remaining text and any parsed time boundaries.
// Supported formats: YYYY, YYYY-MM, YYYY-MM-DD.
func extractDateFilters(query string) (string, *time.Time, *time.Time) {
	var before, after *time.Time
	var remaining []string

	for _, token := range strings.Fields(query) {
		lower := strings.ToLower(token)
		if strings.HasPrefix(lower, "before:") || strings.HasPrefix(lower, "after:") {
			prefix := lower[:strings.Index(lower, ":")+1]
			dateStr := token[len(prefix):]
			if t, ok := parseDateBestEffort(dateStr); ok {
				if prefix == "before:" {
					before = &t
				} else {
					after = &t
				}
			}
			// Always consume date-prefixed tokens — never pass to text filter,
			// even if the date is incomplete (e.g. "after:201" while typing).
			continue
		}
		remaining = append(remaining, token)
	}

	return strings.Join(remaining, " "), before, after
}

// parseDateBestEffort tries to parse s as YYYY, YYYY-MM, or YYYY-MM-DD.
// If the exact string doesn't parse (e.g. "2015-0" while typing "2015-06"),
// it truncates to the longest valid format length so the previous valid date
// stays active while the user is still typing.
func parseDateBestEffort(s string) (time.Time, bool) {
	formats := []struct {
		layout string
		length int
	}{
		{"2006-01-02", 10},
		{"2006-01", 7},
		{"2006", 4},
	}
	for _, f := range formats {
		if len(s) >= f.length {
			if t, err := time.Parse(f.layout, s[:f.length]); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

// itemDate returns the publish date for a list item (VideoItem or PlaylistItem).
func itemDate(item list.Item) time.Time {
	switch it := item.(type) {
	case VideoItem:
		return it.video.PublishedAt
	case PlaylistItem:
		return it.playlist.PublishedAt
	}
	return time.Time{}
}

func (m *Model) filterItems(items []list.Item, preserveOrder bool) []list.Item {
	if m.filterText == "" {
		return items
	}

	query, before, after := extractDateFilters(m.filterText)

	// Apply date filters first
	if before != nil || after != nil {
		var dated []list.Item
		for _, item := range items {
			d := itemDate(item)
			if before != nil && !d.Before(*before) {
				continue
			}
			if after != nil && d.Before(*after) {
				continue
			}
			dated = append(dated, item)
		}
		items = dated
	}

	// If only date filters (no text remaining), return now
	if query == "" {
		return items
	}

	// searchText returns the text to match against, respecting the titles-only toggle.
	// Fuzzy always uses title only (handled separately below).
	searchText := func(item list.Item) string {
		if m.filterTitlesOnly {
			if di, ok := item.(interface{ Title() string }); ok {
				return di.Title()
			}
		}
		return item.FilterValue()
	}

	switch m.filterMode {
	case filterExact:
		// Case-insensitive substring match
		lower := strings.ToLower(query)
		var filtered []list.Item
		for _, item := range items {
			if strings.Contains(strings.ToLower(searchText(item)), lower) {
				filtered = append(filtered, item)
			}
		}
		return filtered

	case filterWords:
		// All words must appear (case-insensitive, any order)
		words := strings.Fields(strings.ToLower(query))
		var filtered []list.Item
		for _, item := range items {
			val := strings.ToLower(searchText(item))
			match := true
			for _, w := range words {
				if !strings.Contains(val, w) {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, item)
			}
		}
		return filtered

	case filterRegex:
		re, err := regexp.Compile("(?i)" + query)
		if err != nil {
			// Invalid regex — show nothing rather than everything
			return nil
		}
		var filtered []list.Item
		for _, item := range items {
			if re.MatchString(searchText(item)) {
				filtered = append(filtered, item)
			}
		}
		return filtered

	default:
		// Fuzzy match against title only — matching the full FilterValue
		// (title + description) produces poor relevance ranking because
		// long descriptions cause false high-score matches.
		targets := make([]string, len(items))
		for i, item := range items {
			if di, ok := item.(interface{ Title() string }); ok {
				targets[i] = di.Title()
			} else {
				targets[i] = item.FilterValue()
			}
		}

		matches := fuzzy.Find(query, targets)
		if preserveOrder {
			// Sort matches by original index to preserve our sort order
			sort.Slice(matches, func(i, j int) bool {
				return matches[i].Index < matches[j].Index
			})
		}
		// When !preserveOrder, keep fuzzy relevance ranking
		filtered := make([]list.Item, len(matches))
		for i, match := range matches {
			filtered[i] = items[match.Index]
		}
		return filtered
	}
}

// --- other helpers ---

func (m *Model) activeList() *list.Model {
	switch m.activeView {
	case viewPlaylists:
		return &m.playlistList
	case viewVideos:
		return &m.videoList
	case viewPlaylistVideos:
		return &m.playlistVideoList
	}
	return &m.playlistList
}

func (m *Model) activeSortField() *sortField {
	switch m.activeView {
	case viewPlaylists:
		return &m.playlistSort
	case viewVideos:
		return &m.videoSort
	case viewPlaylistVideos:
		return &m.playlistVideoSort
	}
	return &m.playlistSort
}

func (m *Model) activeSortDir() *sortDir {
	switch m.activeView {
	case viewPlaylists:
		return &m.playlistSortDir
	case viewVideos:
		return &m.videoSortDir
	case viewPlaylistVideos:
		return &m.playlistVideoSortDir
	}
	return &m.playlistSortDir
}

// toggleSort toggles a sort field+direction on/off. If the same field and
// direction are already active, it clears the sort. Otherwise it activates
// the requested field and direction.
func (m *Model) toggleSort(field sortField, dir sortDir) {
	sf := m.activeSortField()
	sd := m.activeSortDir()
	if *sf == field && *sd == dir {
		*sf = sortNone
		// Toggling sort off while fuzzy filtering → back to relevance
		if m.filterText != "" && m.filterMode == filterFuzzy {
			m.sortOverridesFuzzy = false
		}
	} else {
		*sf = field
		*sd = dir
		// User explicitly chose a sort while fuzzy filtering → override relevance
		if m.filterText != "" && m.filterMode == filterFuzzy {
			m.sortOverridesFuzzy = true
		}
	}
}

func (m *Model) handleTabSwitch() (tea.Model, tea.Cmd) {
	switch m.activeView {
	case viewPlaylists:
		m.activeView = viewVideos
		if m.videoLoadState == loadIdle && m.channel != nil {
			m.videoLoadState = loadLoading
			cmd := m.startVideoFetch(m.channel.LongFormPlaylistID, m.channel.ID, false)
			return m, cmd
		}
	case viewVideos:
		m.activeView = viewPlaylists
	case viewPlaylistVideos:
		// Exit drill view, go to uploads videos
		m.activeView = viewVideos
		m.currentPlaylist = nil
		m.playlistVideos = nil
		m.playlistVideoLoadState = loadIdle
		if m.videoLoadState == loadIdle && m.channel != nil {
			m.videoLoadState = loadLoading
			m.applyFilterAndSort()
			m.updateDetail()
			cmd := m.startVideoFetch(m.channel.LongFormPlaylistID, m.channel.ID, false)
			return m, cmd
		}
	}
	// Re-apply filter to the new view
	m.applyFilterAndSort()
	m.updateDetail()
	return m, nil
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	if m.filtering {
		return m, nil
	}
	selected := m.activeList().SelectedItem()
	if selected == nil {
		return m, nil
	}

	switch item := selected.(type) {
	case PlaylistItem:
		// Drill into the playlist
		p := item.playlist
		m.currentPlaylist = &p
		m.activeView = viewPlaylistVideos
		m.playlistVideoLoadState = loadLoading
		m.playlistVideos = nil
		m.playlistVideoSort = sortNone
		m.filterText = ""
		m.fstate.text = ""
		m.sortOverridesFuzzy = false
		m.playlistVideoList.SetItems(nil)
		m.updateSizes()
		return m, fetchPlaylistVideosCmd(m.ytClient, m.cache, p.ID, p.ChannelID, false)
	case VideoItem:
		return m, openURLCmd(item.URL())
	}
	return m, nil
}

func (m *Model) handleOpenURL() (tea.Model, tea.Cmd) {
	selected := m.activeList().SelectedItem()
	if selected == nil {
		return m, nil
	}

	var url string
	switch item := selected.(type) {
	case PlaylistItem:
		url = item.URL()
	case VideoItem:
		url = item.URL()
	}
	if url != "" {
		return m, openURLCmd(url)
	}
	return m, nil
}

func (m *Model) handleCopyURL() (tea.Model, tea.Cmd) {
	selected := m.activeList().SelectedItem()
	if selected == nil {
		return m, nil
	}

	var url string
	switch item := selected.(type) {
	case PlaylistItem:
		url = item.URL()
	case VideoItem:
		url = item.URL()
	}
	if url != "" {
		m.flashMessage = "Copied!"
		m.flashExpiry = time.Now().Add(1 * time.Second)
		m.fstate.flashIndex = m.activeList().Index()
		m.fstate.flashOn = true
		return m, tea.Batch(
			tea.SetClipboard(url),
			clearFlashAfter(1*time.Second),
		)
	}
	return m, nil
}

func clearFlashAfter(d time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return clearFlashMsg{}
	}
}

func (m *Model) handleBack() (tea.Model, tea.Cmd) {
	m.activeView = viewPlaylists
	m.currentPlaylist = nil
	m.playlistVideos = nil
	m.playlistVideoLoadState = loadIdle
	m.playlistVideoSort = sortNone
	m.filterText = ""
	m.fstate.text = ""
	m.sortOverridesFuzzy = false
	m.applyFilterAndSort()
	m.updateDetail()
	m.updateSizes()
	return m, nil
}

func (m *Model) handleRefresh() (tea.Model, tea.Cmd) {
	if m.channel == nil {
		return m, resolveChannelCmd(m.ytClient, m.cache, m.channelInput)
	}
	switch m.activeView {
	case viewPlaylists:
		m.playlistLoadState = loadLoading
		m.playlists = nil
		m.playlistList.SetItems(nil)
		m.updateDetail()
		return m, fetchPlaylistsCmd(m.ytClient, m.cache, m.channel.ID, true)
	case viewVideos:
		m.videoLoadState = loadLoading
		m.videos = nil
		m.videoList.SetItems(nil)
		m.updateDetail()
		return m, m.startVideoFetch(m.channel.LongFormPlaylistID, m.channel.ID, true)
	case viewPlaylistVideos:
		if m.currentPlaylist != nil {
			m.playlistVideoLoadState = loadLoading
			m.playlistVideos = nil
			m.playlistVideoList.SetItems(nil)
			m.updateDetail()
			return m, fetchPlaylistVideosCmd(m.ytClient, m.cache, m.currentPlaylist.ID, m.currentPlaylist.ChannelID, true)
		}
	}
	return m, nil
}

func (m *Model) startVideoFetch(playlistID, channelID string, skipCache bool) tea.Cmd {
	// Cancel any in-flight fetch
	if m.videoCancel != nil {
		m.videoCancel()
	}
	m.videoFetchGen++
	m.videoTotal = 0
	m.videoLoaded = 0
	ctx, cancel := context.WithCancel(context.Background())
	m.videoCancel = cancel
	progressCh := make(chan videoLoadingMsg, 64)
	m.videoProgressCh = progressCh
	return tea.Batch(
		fetchVideosCmd(m.ytClient, m.cache, playlistID, channelID, m.videoFetchGen, ctx, progressCh, skipCache),
		listenForVideoProgress(progressCh),
	)
}

func (m *Model) clearVideoProgress() {
	m.videoTotal = 0
	m.videoLoaded = 0
	m.videoProgressCh = nil
	m.videoCancel = nil
}

func (m *Model) updateDetail() {
	selected := m.activeList().SelectedItem()
	if selected == nil {
		m.detailViewport.SetContent("")
		return
	}
	content := renderDetail(selected, m.detailWidth(), m.fstate)
	m.detailViewport.SetContent(content)
}

func (m *Model) updateSizes() {
	if m.pickerMode {
		// header + input + blank line + "Recent channels:" label + help = 5
		pickerChrome := 5
		pickerHeight := m.height - pickerChrome
		if pickerHeight < 1 {
			pickerHeight = 1
		}
		m.pickerInput.SetWidth(m.width - 2) // -2 for prompt "> "
		m.pickerList.SetWidth(m.width)
		m.pickerList.SetHeight(pickerHeight)
		return
	}

	headerHeight := 3 // header + tab bar + filter bar
	helpHeight := 1
	contentHeight := m.height - headerHeight - helpHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	if m.showDetail {
		listWidth := m.width * 3 / 5
		detailWidth := m.width - listWidth - 1

		m.playlistList.SetWidth(listWidth)
		m.playlistList.SetHeight(contentHeight)
		m.videoList.SetWidth(listWidth)
		m.videoList.SetHeight(contentHeight)
		m.playlistVideoList.SetWidth(listWidth)
		m.playlistVideoList.SetHeight(contentHeight)
		m.detailViewport.SetWidth(detailWidth)
		m.detailViewport.SetHeight(contentHeight)
	} else {
		m.playlistList.SetWidth(m.width)
		m.playlistList.SetHeight(contentHeight)
		m.videoList.SetWidth(m.width)
		m.videoList.SetHeight(contentHeight)
		m.playlistVideoList.SetWidth(m.width)
		m.playlistVideoList.SetHeight(contentHeight)
	}
}

func (m *Model) detailWidth() int {
	if !m.showDetail {
		return 0
	}
	return m.width - (m.width * 3 / 5) - 1
}

// --- rendering ---

func (m Model) renderPickerView() tea.View {
	var sections []string

	title := headerStyle.Render("yt-browse")
	sections = append(sections, title)
	sections = append(sections, m.pickerInput.View())

	if m.lastError != nil {
		sections = append(sections, errorStyle.Render(fmt.Sprintf("Error: %s", m.lastError)))
	} else if m.pickerResolving {
		sections = append(sections, statusStyle.Render(fmt.Sprintf("Resolving %s...", m.channelInput)))
	}

	if len(m.pickerList.Items()) > 0 {
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render("Recent channels:"))
		sections = append(sections, m.pickerList.View())
	} else if m.pickerInput.Value() == "" && !m.pickerResolving {
		sections = append(sections, "")
		sections = append(sections, statusStyle.Render("No recent channels. Type a channel handle, URL, or ID above."))
	}

	sep := helpDescStyle.Render(" · ")
	helpLine := helpKeyStyle.Render("enter") + helpDescStyle.Render(" select") +
		sep +
		helpKeyStyle.Render("ctrl+o") + helpDescStyle.Render(" lookup as channel") +
		sep +
		helpKeyStyle.Render("↑↓") + helpDescStyle.Render(" navigate") +
		sep +
		helpKeyStyle.Render("ctrl+c") + helpDescStyle.Render(" quit")
	sections = append(sections, helpLine)

	str := lipgloss.JoinVertical(lipgloss.Left, sections...)
	v := tea.NewView(str)
	v.AltScreen = true
	return v
}

func (m Model) renderHeader() string {
	if m.channel == nil {
		if m.lastError != nil {
			return errorStyle.Render(fmt.Sprintf("Error: %s", m.lastError))
		}
		return statusStyle.Render(fmt.Sprintf("Resolving %s...", m.channelInput))
	}
	title := fmt.Sprintf("yt-browse: %s (%s)", m.channel.Handle, m.channel.Title)
	if m.flashMessage != "" {
		return headerStyle.Render(title) + "  " + flashStyle.Render(m.flashMessage)
	}
	return headerStyle.Render(title)
}

func (m Model) renderTabBar() string {
	if m.activeView == viewPlaylistVideos && m.currentPlaylist != nil {
		// Breadcrumb for drill-in view
		var countLabel string
		if m.playlistVideoLoadState == loadDone {
			countLabel = fmt.Sprintf(" (%d videos)", len(m.playlistVideos))
		} else if m.playlistVideoLoadState == loadLoading {
			countLabel = " (loading...)"
		}
		return inactiveTabStyle.Render("← Playlists") + " " +
			activeTabStyle.Render(m.currentPlaylist.Title+countLabel)
	}

	playlistLabel := "Playlists"
	if m.playlistLoadState == loadDone {
		playlistLabel = fmt.Sprintf("Playlists (%d)", len(m.playlists))
	} else if m.playlistLoadState == loadLoading {
		playlistLabel = "Playlists ..."
	}

	videoLabel := "Videos"
	if m.videoLoadState == loadDone {
		videoLabel = fmt.Sprintf("Videos (%d)", len(m.videos))
	} else if m.videoLoadState == loadLoading {
		if m.videoTotal > 0 {
			videoLabel = fmt.Sprintf("Videos (%d/%d)", m.videoLoaded, m.videoTotal)
		} else {
			videoLabel = "Videos ..."
		}
	}

	var playlistTab, videoTab string
	if m.activeView == viewPlaylists {
		playlistTab = activeTabStyle.Render(playlistLabel)
		videoTab = inactiveTabStyle.Render(videoLabel)
	} else {
		playlistTab = inactiveTabStyle.Render(playlistLabel)
		videoTab = activeTabStyle.Render(videoLabel)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, playlistTab, videoTab)
}

func (m Model) renderFilterBar() string {
	modeLabel := m.filterMode.String()

	var modeHint string
	if m.filterMode == filterFuzzy {
		modeHint = filterModeStyle.Render("[" + modeLabel + " · ctrl+t]")
	} else {
		scopeLabel := "titles+desc"
		if m.filterTitlesOnly {
			scopeLabel = "titles"
		}
		modeHint = filterModeStyle.Render("[" + modeLabel + " · " + scopeLabel + " · ctrl+t/ctrl+d]")
	}

	if m.filtering {
		return m.filterInput.View() + "  " + modeHint
	}

	if m.filterText != "" {
		// Show applied filter (not actively editing)
		return filterPromptStyle.Render("  / ") +
			filterTextStyle.Render(m.filterText) +
			"  " + modeHint +
			"  " + helpDescStyle.Render("(esc to clear)")
	}

	// No filter active — show hint
	return helpDescStyle.Render("  / to filter")
}

func (m Model) renderContent() string {
	var listView string
	switch m.activeView {
	case viewPlaylists:
		if m.playlistLoadState == loadLoading && len(m.playlists) == 0 {
			listView = statusStyle.Render("  Loading playlists...")
		} else {
			listView = m.playlistList.View()
		}
	case viewVideos:
		if m.videoLoadState == loadLoading && len(m.videos) == 0 {
			if m.videoTotal > 0 {
				line := fmt.Sprintf("  Loading videos (%d/%d)...", m.videoLoaded, m.videoTotal)
				if m.videoTotal > 500 {
					line += "\n" + fmt.Sprintf("  This may take a bit, but results are cached for %s", formatTTL(m.cfg.CacheTTL))
				}
				listView = statusStyle.Render(line)
			} else {
				listView = statusStyle.Render("  Loading videos...")
			}
		} else {
			listView = m.videoList.View()
		}
	case viewPlaylistVideos:
		if m.playlistVideoLoadState == loadLoading && len(m.playlistVideos) == 0 {
			listView = statusStyle.Render("  Loading playlist videos...")
		} else {
			listView = m.playlistVideoList.View()
		}
	}

	if !m.showDetail {
		return listView
	}

	// Ensure the list side doesn't collapse when empty/no matches
	minListWidth := m.width / 3
	if lipgloss.Width(listView) < minListWidth {
		listView = lipgloss.NewStyle().Width(minListWidth).Render(listView)
	}
	detail := detailBorderStyle.Render(m.detailViewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, detail)
}

// formatTTL renders a duration as a human-friendly string (e.g. "24h", "30m").
func formatTTL(d time.Duration) string {
	if h := int(d.Hours()); h > 0 && d == time.Duration(h)*time.Hour {
		return fmt.Sprintf("%dh", h)
	}
	return d.String()
}

// sortIndicator returns ↓ or ↑ for the given direction.
func sortIndicator(dir sortDir) string {
	if dir == sortAsc {
		return "↑"
	}
	return "↓"
}

// helpItem renders a compact help item where the key letter is highlighted.
// If active is true, the whole thing is rendered in the active sort style with a direction arrow.
func helpItem(key, rest string, active bool, dir sortDir) string {
	if active {
		return sortActiveStyle.Render(key+rest) + sortActiveStyle.Render(sortIndicator(dir))
	}
	return helpKeyStyle.Render(key) + helpDescStyle.Render(rest)
}

// helpItemMid renders a word with a highlighted key in the middle (e.g., d[u]ration).
func helpItemMid(before, key, after string, active bool, dir sortDir) string {
	if active {
		return sortActiveStyle.Render(before+key+after) + sortActiveStyle.Render(sortIndicator(dir))
	}
	return helpDescStyle.Render(before) + helpKeyStyle.Render(key) + helpDescStyle.Render(after)
}

func (m Model) renderHelpBar() string {
	var parts []string
	sep := helpDescStyle.Render(" · ")

	parts = append(parts, helpItem("q", "uit", false, sortDesc))
	parts = append(parts, helpItem("?", " help", false, sortDesc))
	parts = append(parts, helpItem("/", "filter", false, sortDesc))
	if m.filterText != "" {
		parts = append(parts, helpItem("esc", " clear", false, sortDesc))
	}

	switch m.activeView {
	case viewPlaylists:
		parts = append(parts, helpItem("tab", " switch", false, sortDesc))
		parts = append(parts, helpItem("⏎", " view", false, sortDesc))
		parts = append(parts, helpItem("o", "pen", false, sortDesc))
		parts = append(parts, helpItem("y", "ank", false, sortDesc))
		parts = append(parts, helpItem("d", "ate", m.playlistSort == sortByDate, m.playlistSortDir))
		parts = append(parts, helpItem("c", "ount", m.playlistSort == sortByCount, m.playlistSortDir))

	case viewVideos:
		parts = append(parts, helpItem("tab", " switch", false, sortDesc))
		parts = append(parts, helpItem("⏎", " open", false, sortDesc))
		parts = append(parts, helpItem("y", "ank", false, sortDesc))
		parts = append(parts, helpItem("d", "ate", m.videoSort == sortByDate, m.videoSortDir))
		parts = append(parts, helpItem("v", "iews", m.videoSort == sortByViews, m.videoSortDir))
		parts = append(parts, helpItemMid("d", "u", "ration", m.videoSort == sortByDuration, m.videoSortDir))

	case viewPlaylistVideos:
		parts = append(parts, helpItem("⌫", " back", false, sortDesc))
		parts = append(parts, helpItem("⏎", " open", false, sortDesc))
		parts = append(parts, helpItem("y", "ank", false, sortDesc))
		parts = append(parts, helpItem("d", "ate", m.playlistVideoSort == sortByDate, m.playlistVideoSortDir))
		parts = append(parts, helpItem("v", "iews", m.playlistVideoSort == sortByViews, m.playlistVideoSortDir))
		parts = append(parts, helpItemMid("d", "u", "ration", m.playlistVideoSort == sortByDuration, m.playlistVideoSortDir))
	}

	return strings.Join(parts, sep)
}

func (m Model) renderHelpOverlay() string {
	title := helpOverlayTitleStyle.Render("yt-browse keybindings")

	row := func(k, desc string) string {
		return helpOverlayKeyStyle.Render(k) + helpOverlayDescStyle.Render(desc)
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, helpOverlayTitleStyle.Render("Navigation"))
	lines = append(lines, row("tab", "Switch between playlists/videos"))
	lines = append(lines, row("enter", "Open video / drill into playlist"))
	lines = append(lines, row("o", "Open selected item in browser"))
	lines = append(lines, row("y", "Copy URL to clipboard"))
	lines = append(lines, row("backspace", "Back to playlists (from drill view)"))
	lines = append(lines, row("S-enter / O", "Open playlist URL (from drill view)"))
	lines = append(lines, "")
	lines = append(lines, helpOverlayTitleStyle.Render("Filter & Sort"))
	lines = append(lines, row("/", "Start filtering"))
	lines = append(lines, row("esc", "Clear filter"))
	lines = append(lines, row("ctrl+t", "Cycle filter mode (fuzzy/exact/words/regex)"))
	lines = append(lines, row("ctrl+d", "Toggle title-only / title+description search"))
	lines = append(lines, row("before/after:", "Date filter (YYYY, YYYY-MM, YYYY-MM-DD)"))
	lines = append(lines, row("d / D", "Sort by date (newest / oldest)"))
	lines = append(lines, row("v / V", "Sort by views (most / fewest)"))
	lines = append(lines, row("u / U", "Sort by duration (longest / shortest)"))
	lines = append(lines, row("c / C", "Sort by count (most / fewest, playlists)"))
	lines = append(lines, "")
	lines = append(lines, helpOverlayTitleStyle.Render("Other"))
	lines = append(lines, row("r", "Refresh data from API"))
	lines = append(lines, row("q / ctrl+c", "Quit"))
	lines = append(lines, "")
	lines = append(lines, helpDescStyle.Render("Press any key to close"))

	content := strings.Join(lines, "\n")
	box := helpOverlayStyle.Render(content)

	// Center the overlay
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
