package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/nroyalty/yt-browse/internal/cache"
	"github.com/nroyalty/yt-browse/internal/config"
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
)

type Model struct {
	cfg      *config.Config
	ytClient *youtube.Client
	cache    *cache.Store
	keys     keyMap

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
	currentPlaylist         *youtube.Playlist
	playlistVideoList       list.Model
	playlistVideoLoadState  loadState
	playlistVideos          []youtube.Video
	playlistVideoSort       sortField
	playlistVideoSortDir    sortDir

	// Filter: we manage our own filter instead of using the list's built-in one
	filterInput textinput.Model
	filtering   bool       // is the filter input active/focused
	filterText  string     // the applied filter text (persists after closing input)
	filterMode  filterMode // fuzzy or exact
	fstate      *filterState // shared with delegate for match highlighting

	// Detail
	detailViewport  viewport.Model
	showDetail      bool

	// Help overlay
	showHelp bool

	// Error
	lastError error
}

func New(cfg *config.Config, ytClient *youtube.Client, cacheStore *cache.Store, channelInput string) Model {
	fs := &filterState{}
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

	fi := textinput.New()
	fi.Prompt = "/ "
	styles := fi.Styles()
	styles.Focused.Prompt = filterPromptStyle
	styles.Focused.Text = filterTextStyle
	fi.SetStyles(styles)

	vp := viewport.New()

	return Model{
		cfg:            cfg,
		ytClient:       ytClient,
		cache:          cacheStore,
		keys:           defaultKeyMap(),
		channelInput:   channelInput,
		activeView:     viewPlaylists,
		playlistList:      playlistList,
		videoList:         videoList,
		playlistVideoList: playlistVideoList,
		filterInput:       fi,
		filterMode:     filterFuzzy,
		fstate:         fs,
		detailViewport: vp,
		showDetail:     true,
		playlistSort:   sortNone,
		videoSort:      sortByDate,
		videoSortDir:   sortDesc,
	}
}

func (m Model) Init() tea.Cmd {
	return resolveChannelCmd(m.ytClient, m.cache, m.channelInput)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()
		m.updateDetail()
		return m, nil

	case tea.KeyPressMsg:
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

		case key.Matches(msg, m.keys.OpenPlaylist):
			if m.activeView == viewPlaylistVideos && m.currentPlaylist != nil {
				return m, openURLCmd(m.currentPlaylist.URL())
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleFilterMode):
			if m.filterMode == filterFuzzy {
				m.filterMode = filterExact
			} else {
				m.filterMode = filterFuzzy
			}
			if m.filterText != "" {
				m.applyFilterAndSort()
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

		case key.Matches(msg, m.keys.Refresh):
			return m.handleRefresh()

		}

	case channelResolvedMsg:
		m.channel = msg.channel
		m.playlistLoadState = loadLoading
		return m, fetchPlaylistsCmd(m.ytClient, m.cache, m.channel.ID, false)

	case channelErrorMsg:
		m.lastError = msg.err
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

	}

	// Delegate to active list
	var cmd tea.Cmd
	switch m.activeView {
	case viewPlaylists:
		m.playlistList, cmd = m.playlistList.Update(msg)
	case viewVideos:
		m.videoList, cmd = m.videoList.Update(msg)
	case viewPlaylistVideos:
		m.playlistVideoList, cmd = m.playlistVideoList.Update(msg)
	}
	cmds = append(cmds, cmd)

	// Update detail pane
	m.updateDetail()

	return m, tea.Batch(cmds...)
}

func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var sections []string

	sections = append(sections, m.renderHeader())
	sections = append(sections, m.renderTabBar())

	// Filter bar (shown when filtering or filter is active)
	if m.filtering || m.filterText != "" {
		sections = append(sections, m.renderFilterBar())
	}

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

// --- filter handling ---

func (m *Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Apply filter and close input
		m.filterText = m.filterInput.Value()
		m.filtering = false
		m.filterInput.Blur()
		m.applyFilterAndSort()
		m.updateSizes() // filter bar may have changed visibility
		return m, nil
	case "esc":
		// Cancel: revert to previous filter text
		m.filtering = false
		m.filterInput.Blur()
		m.filterInput.SetValue(m.filterText)
		m.updateSizes()
		return m, nil
	default:
		// Forward to textinput
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		// Live-filter as user types
		m.filterText = m.filterInput.Value()
		m.applyFilterAndSort()
		return m, cmd
	}
}

// applyFilterAndSort is the core pipeline: raw data → sort → filter → SetItems
func (m *Model) applyFilterAndSort() {
	// Sync filter state to the delegate for match highlighting
	m.fstate.text = m.filterText
	m.fstate.mode = m.filterMode

	switch m.activeView {
	case viewPlaylists:
		items := m.sortedPlaylistItems()
		if m.filterText != "" {
			items = m.filterItems(items, m.playlistSort != sortNone)
		}
		m.playlistList.SetItems(items)
	case viewVideos:
		items := m.sortedVideoItems(m.videos, m.videoSort, m.videoSortDir)
		if m.filterText != "" {
			items = m.filterItems(items, m.videoSort != sortNone)
		}
		m.videoList.SetItems(items)
	case viewPlaylistVideos:
		items := m.sortedVideoItems(m.playlistVideos, m.playlistVideoSort, m.playlistVideoSortDir)
		if m.filterText != "" {
			items = m.filterItems(items, m.playlistVideoSort != sortNone)
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

func (m *Model) filterItems(items []list.Item, preserveOrder bool) []list.Item {
	if m.filterText == "" {
		return items
	}

	query := m.filterText

	if m.filterMode == filterExact {
		// Case-insensitive substring match
		lower := strings.ToLower(query)
		var filtered []list.Item
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.FilterValue()), lower) {
				filtered = append(filtered, item)
			}
		}
		return filtered
	}

	// Fuzzy match using sahilm/fuzzy
	targets := make([]string, len(items))
	for i, item := range items {
		targets[i] = item.FilterValue()
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
	} else {
		*sf = field
		*sd = dir
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

func (m *Model) handleBack() (tea.Model, tea.Cmd) {
	m.activeView = viewPlaylists
	m.currentPlaylist = nil
	m.playlistVideos = nil
	m.playlistVideoLoadState = loadIdle
	m.playlistVideoSort = sortNone
	m.filterText = ""
	m.fstate.text = ""
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
	content := renderDetail(selected, m.detailWidth())
	m.detailViewport.SetContent(content)
}

func (m *Model) updateSizes() {
	headerHeight := 2 // header + tab bar
	if m.filtering || m.filterText != "" {
		headerHeight++ // filter bar
	}
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

func (m Model) renderHeader() string {
	if m.channel == nil {
		if m.lastError != nil {
			return errorStyle.Render(fmt.Sprintf("Error: %s", m.lastError))
		}
		return statusStyle.Render(fmt.Sprintf("Resolving %s...", m.channelInput))
	}
	title := fmt.Sprintf("yt-browse: %s (%s)", m.channel.Handle, m.channel.Title)
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
		total := len(m.videos)
		shown := len(m.videoList.Items())
		if shown < total {
			videoLabel = fmt.Sprintf("Videos (%d/%d)", shown, total)
		} else {
			videoLabel = fmt.Sprintf("Videos (%d)", total)
		}
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
	modeLabel := "fuzzy"
	if m.filterMode == filterExact {
		modeLabel = "exact"
	}

	if m.filtering {
		return m.filterInput.View() + "  " + filterModeStyle.Render("["+modeLabel+"]")
	}

	// Show applied filter (not actively editing)
	return filterPromptStyle.Render("/ ") +
		filterTextStyle.Render(m.filterText) +
		"  " + filterModeStyle.Render("["+modeLabel+"]") +
		"  " + helpDescStyle.Render("(esc to clear)")
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
				listView = statusStyle.Render(fmt.Sprintf("  Loading videos (%d/%d)...", m.videoLoaded, m.videoTotal))
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

	detail := detailBorderStyle.Render(m.detailViewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, detail)
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

	// Filter status
	parts = append(parts, helpItem("/", "filter", false, sortDesc))
	if m.filterText != "" {
		parts = append(parts, helpItem("esc", " clear", false, sortDesc))
	}

	switch m.activeView {
	case viewPlaylists:
		parts = append(parts, helpItem("tab", " switch", false, sortDesc))
		parts = append(parts, helpItem("⏎", " view", false, sortDesc))
		parts = append(parts, helpItem("o", "pen", false, sortDesc))
		parts = append(parts, helpItem("d", "ate", m.playlistSort == sortByDate, m.playlistSortDir))

	case viewVideos:
		parts = append(parts, helpItem("tab", " switch", false, sortDesc))
		parts = append(parts, helpItem("⏎", " open", false, sortDesc))
		parts = append(parts, helpItem("d", "ate", m.videoSort == sortByDate, m.videoSortDir))
		parts = append(parts, helpItem("v", "iews", m.videoSort == sortByViews, m.videoSortDir))
		parts = append(parts, helpItemMid("d", "u", "ration", m.videoSort == sortByDuration, m.videoSortDir))

	case viewPlaylistVideos:
		parts = append(parts, helpItem("⌫", " back", false, sortDesc))
		parts = append(parts, helpItem("⏎", " open", false, sortDesc))
		parts = append(parts, helpItem("d", "ate", m.playlistVideoSort == sortByDate, m.playlistVideoSortDir))
		parts = append(parts, helpItem("v", "iews", m.playlistVideoSort == sortByViews, m.playlistVideoSortDir))
		parts = append(parts, helpItemMid("d", "u", "ration", m.playlistVideoSort == sortByDuration, m.playlistVideoSortDir))
	}

	parts = append(parts, helpItem("?", " help", false, sortDesc))
	parts = append(parts, helpItem("q", "uit", false, sortDesc))

	return strings.Join(parts, sep)
}

func (m Model) renderHelpOverlay() string {
	title := helpOverlayTitleStyle.Render("Keybindings")

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
	lines = append(lines, row("backspace", "Back to playlists (from drill view)"))
	lines = append(lines, row("S-enter / O", "Open playlist URL (from drill view)"))
	lines = append(lines, "")
	lines = append(lines, helpOverlayTitleStyle.Render("Filter & Sort"))
	lines = append(lines, row("/", "Start filtering"))
	lines = append(lines, row("esc", "Clear filter"))
	lines = append(lines, row("ctrl+f", "Toggle fuzzy / exact filter"))
	lines = append(lines, row("d / D", "Sort by date (newest / oldest)"))
	lines = append(lines, row("v / V", "Sort by views (most / fewest)"))
	lines = append(lines, row("u / U", "Sort by duration (longest / shortest)"))
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
