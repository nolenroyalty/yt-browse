package tui

import (
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

	// Videos (uploads)
	videoList      list.Model
	videoLoadState loadState
	videos         []youtube.Video
	videoSort      sortField

	// Playlist videos (drill-in)
	currentPlaylist         *youtube.Playlist
	playlistVideoList       list.Model
	playlistVideoLoadState  loadState
	playlistVideos          []youtube.Video
	playlistVideoSort       sortField

	// Filter: we manage our own filter instead of using the list's built-in one
	filterInput textinput.Model
	filtering   bool       // is the filter input active/focused
	filterText  string     // the applied filter text (persists after closing input)
	filterMode  filterMode // fuzzy or exact
	fstate      *filterState // shared with delegate for match highlighting

	// Detail
	detailViewport viewport.Model
	showDetail     bool

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
		videoSort:      sortNone,
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
		return m, nil

	case tea.KeyPressMsg:
		// When filter input is active, handle keys for the filter
		if m.filtering {
			return m.handleFilterKey(msg)
		}

		switch {
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
			sf := m.activeSortField()
			if *sf == sortByDate {
				*sf = sortNone
			} else {
				*sf = sortByDate
			}
			m.applyFilterAndSort()
			m.updateDetail()
			return m, nil

		case key.Matches(msg, m.keys.SortViews):
			if m.activeView == viewVideos || m.activeView == viewPlaylistVideos {
				sf := m.activeSortField()
				if *sf == sortByViews {
					*sf = sortNone
				} else {
					*sf = sortByViews
				}
				m.applyFilterAndSort()
				m.updateDetail()
			}
			return m, nil

		case key.Matches(msg, m.keys.SortDuration):
			if m.activeView == viewVideos || m.activeView == viewPlaylistVideos {
				sf := m.activeSortField()
				if *sf == sortByDuration {
					*sf = sortNone
				} else {
					*sf = sortByDuration
				}
				m.applyFilterAndSort()
				m.updateDetail()
			}
			return m, nil

		case key.Matches(msg, m.keys.Refresh):
			return m.handleRefresh()
		}

	case channelResolvedMsg:
		m.channel = msg.channel
		m.playlistLoadState = loadLoading
		return m, fetchPlaylistsCmd(m.ytClient, m.cache, m.channel.ID)

	case channelErrorMsg:
		m.lastError = msg.err
		return m, nil

	case playlistsFetchedMsg:
		m.playlists = msg.playlists
		m.playlistLoadState = loadDone
		m.applyFilterAndSort()
		m.updateDetail()
		// Start fetching videos in background
		if m.channel != nil && m.channel.UploadsPlaylistID != "" {
			m.videoLoadState = loadLoading
			cmds = append(cmds, fetchVideosCmd(m.ytClient, m.cache, m.channel.UploadsPlaylistID, m.channel.ID))
		}
		return m, tea.Batch(cmds...)

	case playlistsErrorMsg:
		m.playlistLoadState = loadError
		m.lastError = msg.err
		return m, nil

	case videosFetchedMsg:
		m.videos = msg.videos
		m.videoLoadState = loadDone
		m.applyFilterAndSort()
		m.updateDetail()
		return m, nil

	case videosErrorMsg:
		m.videoLoadState = loadError
		m.lastError = msg.err
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

	str := lipgloss.JoinVertical(lipgloss.Left, sections...)

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
		items := m.sortedVideoItems(m.videos, m.videoSort)
		if m.filterText != "" {
			items = m.filterItems(items, m.videoSort != sortNone)
		}
		m.videoList.SetItems(items)
	case viewPlaylistVideos:
		items := m.sortedVideoItems(m.playlistVideos, m.playlistVideoSort)
		if m.filterText != "" {
			items = m.filterItems(items, m.playlistVideoSort != sortNone)
		}
		m.playlistVideoList.SetItems(items)
	}
}

func (m *Model) sortedPlaylistItems() []list.Item {
	sorted := make([]youtube.Playlist, len(m.playlists))
	copy(sorted, m.playlists)

	switch m.playlistSort {
	case sortByDate:
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].PublishedAt.After(sorted[j].PublishedAt) })
	}

	items := make([]list.Item, len(sorted))
	for i, p := range sorted {
		items[i] = PlaylistItem{playlist: p}
	}
	return items
}

func (m *Model) sortedVideoItems(videos []youtube.Video, sortBy sortField) []list.Item {
	sorted := make([]youtube.Video, len(videos))
	copy(sorted, videos)

	switch sortBy {
	case sortByDate:
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].PublishedAt.After(sorted[j].PublishedAt) })
	case sortByViews:
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].ViewCount > sorted[j].ViewCount })
	case sortByDuration:
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Duration > sorted[j].Duration })
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

func (m *Model) handleTabSwitch() (tea.Model, tea.Cmd) {
	switch m.activeView {
	case viewPlaylists:
		m.activeView = viewVideos
		if m.videoLoadState == loadIdle && m.channel != nil {
			m.videoLoadState = loadLoading
			return m, fetchVideosCmd(m.ytClient, m.cache, m.channel.UploadsPlaylistID, m.channel.ID)
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
			return m, fetchVideosCmd(m.ytClient, m.cache, m.channel.UploadsPlaylistID, m.channel.ID)
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
		return m, fetchPlaylistVideosCmd(m.ytClient, m.cache, p.ID, p.ChannelID)
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
		return m, fetchPlaylistsCmd(m.ytClient, m.cache, m.channel.ID)
	case viewVideos:
		m.videoLoadState = loadLoading
		return m, fetchVideosCmd(m.ytClient, m.cache, m.channel.UploadsPlaylistID, m.channel.ID)
	case viewPlaylistVideos:
		if m.currentPlaylist != nil {
			m.playlistVideoLoadState = loadLoading
			return m, fetchPlaylistVideosCmd(m.ytClient, m.cache, m.currentPlaylist.ID, m.currentPlaylist.ChannelID)
		}
	}
	return m, nil
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
		videoLabel = fmt.Sprintf("Videos (%d)", len(m.videos))
	} else if m.videoLoadState == loadLoading {
		videoLabel = "Videos ..."
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
			listView = statusStyle.Render("  Loading videos...")
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

func (m Model) renderHelpBar() string {
	var parts []string

	add := func(k, desc string) {
		parts = append(parts, helpKeyStyle.Render(k)+" "+helpDescStyle.Render(desc))
	}

	add("/", "filter")
	if m.filterText != "" {
		add("esc", "clear")
	}
	add("ctrl+f", "fuzzy/exact")

	type sortOption struct {
		key   string
		label string
		field sortField
	}

	switch m.activeView {
	case viewPlaylists:
		add("tab", "switch")
		add("enter", "view")
		add("o", "open")
		currentSort := m.playlistSort
		opts := []sortOption{
			{"d", "date", sortByDate},
		}
		for _, o := range opts {
			if o.field == currentSort {
				parts = append(parts, sortActiveStyle.Render(o.key+" "+o.label+"*"))
			} else {
				add(o.key, o.label)
			}
		}

	case viewVideos:
		add("tab", "switch")
		add("enter", "open")
		currentSort := m.videoSort
		opts := []sortOption{
			{"d", "date", sortByDate},
			{"v", "views", sortByViews},
			{"u", "duration", sortByDuration},
		}
		for _, o := range opts {
			if o.field == currentSort {
				parts = append(parts, sortActiveStyle.Render(o.key+" "+o.label+"*"))
			} else {
				add(o.key, o.label)
			}
		}

	case viewPlaylistVideos:
		add("bksp", "back")
		add("enter", "open")
		add("S-enter", "open playlist")
		currentSort := m.playlistVideoSort
		opts := []sortOption{
			{"d", "date", sortByDate},
			{"v", "views", sortByViews},
			{"u", "duration", sortByDuration},
		}
		for _, o := range opts {
			if o.field == currentSort {
				parts = append(parts, sortActiveStyle.Render(o.key+" "+o.label+"*"))
			} else {
				add(o.key, o.label)
			}
		}
	}

	add("r", "refresh")
	add("q", "quit")

	return strings.Join(parts, helpDescStyle.Render("  |  "))
}
