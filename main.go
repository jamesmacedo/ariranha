package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ariranha/appdir"
	"ariranha/gallery"
	"ariranha/search"
	"ariranha/ui"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func searchStatus(st search.Status) string {
	var parts []string
	if st.Reading {
		if st.Discovered == 0 {
			parts = append(parts, "looking for photos…")
		} else {
			parts = append(parts, fmt.Sprintf("reading %d/%d", st.Cataloged, st.Discovered))
		}
	}
	switch st.State {
	case search.StateLoading:
		parts = append(parts, "loading models…")
	case search.StateIndexing:
		if st.Loading {
			parts = append(parts, "loading models…")
			break
		}
		if st.Paused {
			parts = append(parts, fmt.Sprintf("paused  %d/%d indexed", st.Indexed, max(st.Total, st.Cataloged)), "click to start")
			break
		}
		parts = append(parts, fmt.Sprintf("indexing %d/%d", st.Indexed, max(st.Total, st.Cataloged)))
		parts = append(parts, fmt.Sprintf("%d faces", st.Faces))
		if st.Remaining > 0 {
			parts = append(parts, formatRemaining(st.Remaining))
		}
	case search.StateReady:
		if st.FaceErr != nil {
			parts = append(parts, "faces unavailable: "+st.FaceErr.Error())
		}
	}
	return strings.Join(parts, "  •  ")
}

func formatRemaining(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than 1 min left"
	case d < time.Hour:
		return fmt.Sprintf("~%d min left", int(d.Round(time.Minute).Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("~%dh%02d left", h, m)
	}
}

func setupLog(dirs appdir.Dirs) func() {
	f, err := os.OpenFile(dirs.LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	return func() { f.Close() }
}

func main() {
	dirs, err := appdir.Locate()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := dirs.Ensure(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	closeLog := setupLog(dirs)
	defer closeLog()
	log.Printf("raiz %s", dirs.Root)
	gallery.SetToolsDir(dirs.Runtime)
	gallery.SetDiskCache(filepath.Join(dirs.Cache, "thumbs"))
	if bin, err := gallery.Tool("ffprobe"); err != nil {
		log.Printf("vídeos desativados: %v", err)
	} else {
		log.Printf("ffprobe %s", bin)
	}

	rl.SetConfigFlags(rl.FlagWindowResizable | rl.FlagVsyncHint | rl.FlagMsaa4xHint)
	rl.SetTraceLogLevel(rl.LogWarning)
	rl.InitWindow(1280, 800, "Ariranha")
	defer rl.CloseWindow()

	rl.SetWindowMinSize(640, 480)
	rl.SetTargetFPS(60)
	rl.SetExitKey(rl.KeyNull)

	theme := ui.LoadTheme()
	defer theme.Unload()

	thumbs := gallery.NewThumbCache(360, 400, 256<<20)
	defer thumbs.Close()

	paths := search.NewPaths(dirs.Models, dirs.Runtime, dirs.Database())
	paths.Trash = dirs.Trash
	finder := search.Start(dirs.Storage, paths)
	defer finder.Close()
	libVersion := finder.LibraryVersion()
	lib := finder.Library()
	var libRefreshed time.Time
	var loggedErr, loggedFaceErr, loggedCatalog bool

	albumList := finder.Albums()
	albumsVersion := finder.AlbumsVersion()
	var openAlbum int64
	grid := ui.NewAlbumGrid(theme, thumbs)
	grid.BottomInset = ui.NavInset
	grid.Albums = albumList
	editor := ui.NewAlbumEditor(theme, search.MaxAlbumName, search.MaxAlbumDescription)
	picker := ui.NewAlbumPicker(theme, thumbs)
	picker.Albums = albumList
	selBar := ui.NewSelectionBar(theme)
	toast := &ui.Toast{}
	var pending []*gallery.Photo
	fromSelection := false

	people := ui.NewPeopleStrip(theme)
	people.Empty = "No people recognized yet"
	defer people.Unload()
	peopleVersion := -1

	timeline := ui.NewTimeline(theme, thumbs)
	timeline.BottomInset = ui.NavInset

	backdrop := ui.NewBackdrop()
	defer backdrop.Unload()

	input := ui.NewTextInput(theme, "Search")
	input.Hint = "Ctrl K"
	nav := ui.NewNavBar(theme, input)
	searching := false

	viewer := ui.NewViewer(theme, thumbs)
	defer viewer.Unload()
	viewer.Avatars = people
	viewer.AlbumsOf = finder.AlbumsOf
	viewer.OnFavorite = finder.SetFavorite
	viewer.OnDelete = func(p *gallery.Photo) error {
		if err := finder.Trash(p); err != nil {
			return err
		}
		log.Printf("lixeira: %s", p.Rel)
		timeline.Remove(p)
		return nil
	}
	viewer.PeopleOf = func(p *gallery.Photo) []search.PersonInfo {
		var out []search.PersonInfo
		for _, pi := range people.People {
			for _, rel := range pi.Paths {
				if rel == p.Rel {
					out = append(out, pi)
					break
				}
			}
		}
		return out
	}
	timeline.OnOpen = viewer.Show
	var reveal *gallery.Photo
	viewer.OnClose = func(p *gallery.Photo) { reveal = p }

	personName := func(p search.PersonInfo) string {
		if p.Name != "" {
			return p.Name
		}
		return fmt.Sprintf("Person %d", p.ID)
	}

	findAlbum := func(id int64) *gallery.Album {
		for _, a := range albumList {
			if a.ID == id {
				return a
			}
		}
		return nil
	}
	notify := func(msg string) {
		if viewer.Open {
			viewer.Notify(msg)
		} else {
			toast.Show(msg)
		}
	}
	added := func(a *gallery.Album, n, total int) {
		switch {
		case n == 0 && total == 1:
			notify("Already in " + a.Name)
		case n == 0:
			notify("All already in " + a.Name)
		case n == 1:
			notify("Added 1 item to " + a.Name)
		default:
			notify(fmt.Sprintf("Added %d items to %s", n, a.Name))
		}
	}
	addPending := func(a *gallery.Album) {
		n, err := finder.AddToAlbum(a.ID, pending)
		if err != nil {
			notify("Error: " + err.Error())
			return
		}
		added(a, n, len(pending))
		if fromSelection {
			timeline.ClearSelection()
		}
		pending = nil
	}
	openPicker := func(photos []*gallery.Photo, selection bool) {
		pending = photos
		fromSelection = selection
		picker.Albums = albumList
		picker.Show(len(photos))
	}
	nextColor := func() int {
		return len(albumList) % ui.AlbumColors()
	}

	showView := func() {
		timeline.Accent = rl.Color{}
		timeline.Status = ""
		switch nav.View {
		case ui.ViewPhotos:
			timeline.SetPhotos("Photos", lib.Photos)
		case ui.ViewPeople:
			if p, ok := people.SelectedPerson(); ok {
				timeline.SetPhotos(personName(p), finder.PhotosOf(p))
			} else {
				timeline.SetPhotos("People", nil)
			}
		case ui.ViewAlbums:
			if a := findAlbum(openAlbum); a != nil {
				timeline.SetPhotos(a.Name, a.Photos)
				timeline.Accent = theme.AlbumColor(a.Color)
				timeline.Status = a.Description
			} else {
				openAlbum = 0
				timeline.SetPhotos("Albums", nil)
			}
		}
	}

	refreshAlbums := func() {
		albumList = finder.Albums()
		grid.Albums = albumList
		picker.Albums = albumList
		if nav.View != ui.ViewAlbums || openAlbum == 0 || searching {
			return
		}
		a := findAlbum(openAlbum)
		if a == nil {
			openAlbum = 0
			return
		}
		timeline.Refresh(a.Photos)
		timeline.Title = a.Name
		timeline.Accent = theme.AlbumColor(a.Color)
		timeline.Status = a.Description
	}

	viewer.OnAlbum = func(p *gallery.Photo) {
		openPicker([]*gallery.Photo{p}, false)
	}
	picker.OnPick = addPending
	picker.OnNew = func() {
		editor.New(nextColor())
	}
	grid.OnNew = func() {
		pending = nil
		editor.New(nextColor())
	}
	grid.OnOpen = func(a *gallery.Album) {
		openAlbum = a.ID
		showView()
	}
	editor.OnSave = func(id int64, name, desc string, color int) error {
		if id != 0 {
			if err := finder.UpdateAlbum(id, name, desc, color); err != nil {
				return err
			}
			notify("Album saved")
			return nil
		}
		nid, err := finder.CreateAlbum(name, desc, color)
		if err != nil {
			return err
		}
		refreshAlbums()
		if a := findAlbum(nid); a != nil && len(pending) > 0 {
			addPending(a)
		} else {
			notify("Album created")
		}
		pending = nil
		return nil
	}
	editor.OnDelete = func(id int64) error {
		if err := finder.DeleteAlbum(id); err != nil {
			return err
		}
		if openAlbum == id {
			openAlbum = 0
			showView()
		}
		notify("Album deleted")
		return nil
	}

	enterView := func() {
		searching = false
		switch nav.View {
		case ui.ViewPeople:
			if _, ok := people.SelectedPerson(); !ok && len(people.People) > 0 {
				people.Selected = people.People[0].ID
			}
		case ui.ViewAlbums:
			openAlbum = 0
		}
		showView()
	}
	showView()

	input.OnSubmit = func(q string) {
		if strings.TrimSpace(q) == "" {
			nav.View = ui.ViewPhotos
			enterView()
			return
		}
		finder.Query(q)
	}

	showAll := ui.NewButton(0, 0, 120, 34, "Show all", func() {
		input.Text = ""
		enterView()
	})
	showAll.Font = &theme.Medium
	showAll.Style = theme.ButtonStyle()

	backAlbums := ui.NewButton(0, 0, 110, 34, "‹  Albums", func() {
		openAlbum = 0
		showView()
	})
	backAlbums.Font = &theme.Medium
	backAlbums.Style = theme.ButtonStyle()
	editAlbum := ui.NewButton(0, 0, 80, 34, "Edit", func() {
		if a := findAlbum(openAlbum); a != nil {
			editor.Edit(a)
		}
	})
	editAlbum.Font = &theme.Medium
	editAlbum.Style = theme.ButtonStyle()

	for !rl.WindowShouldClose() {
		w := float32(rl.GetScreenWidth())
		h := float32(rl.GetScreenHeight())
		screen := rl.NewRectangle(0, 0, w, h)

		modal := editor.Open || picker.Open
		var topH float32
		if nav.View == ui.ViewPeople && len(people.People) > 0 {
			topH = ui.PeopleHeight
		}
		top := rl.NewRectangle(0, 0, w, topH)
		body := rl.NewRectangle(0, topH, w, h-topH)

		thumbs.BeginFrame()

		status := finder.Status()
		if status.Err != nil && !loggedErr {
			log.Printf("busca: %v", status.Err)
			loggedErr = true
		}
		if status.FaceErr != nil && !loggedFaceErr {
			log.Printf("rostos: %v", status.FaceErr)
			loggedFaceErr = true
		}
		if r, ok := finder.Poll(); ok {
			nav.View = ui.ViewPhotos
			searching = true
			if r.Err != nil {
				timeline.SetResults("“"+r.Query+"”", "Error: "+r.Err.Error(), nil)
			} else {
				timeline.SetResults("“"+r.Query+"”", fmt.Sprintf("Most relevant  ·  %d ms", r.Took.Milliseconds()), r.Photos)
			}
		}
		if v := finder.PeopleVersion(); v != peopleVersion {
			peopleVersion = v
			people.SetPeople(finder.People())
			if nav.View == ui.ViewPeople && people.Selected == 0 && len(people.People) > 0 {
				enterView()
			}
		}
		if v := finder.LibraryVersion(); v != libVersion && time.Since(libRefreshed) > 400*time.Millisecond {
			libVersion = v
			libRefreshed = time.Now()
			lib = finder.Library()
			refreshAlbums()
			switch {
			case searching:
			case nav.View == ui.ViewPhotos:
				timeline.Refresh(lib.Photos)
			case nav.View == ui.ViewPeople:
				if p, ok := people.SelectedPerson(); ok {
					timeline.Refresh(finder.PhotosOf(p))
				}
			}
		}
		if v := finder.AlbumsVersion(); v != albumsVersion {
			albumsVersion = v
			refreshAlbums()
		}
		if !status.Reading && !loggedCatalog {
			log.Printf("%d fotos em %s", status.Cataloged, dirs.Relative(dirs.Storage))
			loggedCatalog = true
		}

		switch {
		case nav.View == ui.ViewPeople && len(people.People) == 0:
			timeline.EmptyText = people.Empty
		case nav.View == ui.ViewPeople:
			timeline.EmptyText = "Pick a person above"
		case nav.View == ui.ViewAlbums:
			timeline.EmptyText = "This album is empty. Add photos from the viewer or select them in Photos"
		case searching:
			timeline.EmptyText = "No photos found"
		case status.Reading:
			timeline.EmptyText = "Reading ./storage…"
		default:
			timeline.EmptyText = "Put photos in ./storage"
		}
		nav.Status = searchStatus(status)
		nav.Done = false
		nav.Progress = 0
		switch {
		case status.Loading:
			nav.Progress = -1
		case status.State == search.StateIndexing:
			if total := max(status.Total, status.Cataloged); total > 0 {
				nav.Progress = max(0.02, float32(status.Indexed)/float32(total))
			}
		case status.State == search.StateLoading || status.Reading:
			nav.Progress = -1
		case status.State == search.StateReady:
			nav.Done = true
			if nav.Status == "" {
				nav.Status = fmt.Sprintf("%d photos indexed", status.Indexed)
			}
		case status.Err != nil:
			nav.Status = "Search unavailable: " + status.Err.Error()
		}
		nav.Playable = status.State == search.StateLoading || status.State == search.StateIndexing
		nav.Paused = status.Paused
		input.Placeholder = "Search"
		if status.State == search.StateUnavailable {
			input.Placeholder = "Search unavailable"
		}

		toast.Update()
		if viewer.Open {
			viewer.Inert = modal
			viewer.Update(screen)
			if modal {
				picker.Update(screen)
				editor.Update(screen)
			}
		}
		if viewer.Open {
			switch {
			case editor.InputHovered():
				rl.SetMouseCursor(rl.MouseCursorIBeam)
			case modal && (picker.Hovered() || editor.Hovered()), !modal && viewer.Hovered():
				rl.SetMouseCursor(rl.MouseCursorPointingHand)
			default:
				rl.SetMouseCursor(rl.MouseCursorDefault)
			}
			rl.BeginDrawing()
			backdrop.Begin()
			viewer.Draw()
			backdrop.End()
			viewer.DrawOverlay(backdrop)
			picker.Draw(screen)
			editor.Draw(screen)
			rl.EndDrawing()
			continue
		}

		gridMode := nav.View == ui.ViewAlbums && openAlbum == 0 && !searching
		albumOpen := nav.View == ui.ViewAlbums && openAlbum != 0 && !searching
		selecting := !gridMode && timeline.SelectionCount() > 0
		barR := rl.NewRectangle(body.X, body.Y, body.Width, ui.TimelineBar)
		selBar.Count = timeline.SelectionCount()
		if albumOpen {
			selBar.SetActions("Add to album", "Remove from album")
		} else {
			selBar.SetActions("Add to album")
		}

		if modal {
			picker.Update(screen)
			editor.Update(screen)
		} else {
			if nav.Update(screen) {
				input.Text = ""
				input.Focused = false
				enterView()
			}
			if nav.Toggled {
				finder.SetPaused(!status.Paused)
				log.Printf("indexação %s", map[bool]string{true: "retomada", false: "pausada"}[status.Paused])
			}

			switch {
			case gridMode:
				grid.Obstacle = nav.Bounds()
				grid.Update(body)
			default:
				if topH > 0 && people.Update(top) {
					showView()
				}
				if selecting {
					switch clicked, cleared := selBar.Update(barR); {
					case cleared:
						timeline.ClearSelection()
					case clicked == 0:
						openPicker(timeline.Selected(), true)
					case clicked == 1:
						sel := timeline.Selected()
						if n, err := finder.RemoveFromAlbum(openAlbum, sel); err != nil {
							notify("Error: " + err.Error())
						} else {
							notify(fmt.Sprintf("Removed %d from album", n))
						}
						timeline.ClearSelection()
					}
				} else if albumOpen && !input.Focused && rl.IsKeyPressed(rl.KeyEscape) {
					openAlbum = 0
					showView()
				}
				timeline.KeysEnabled = !input.Focused
				timeline.Obstacle = nav.Bounds()
				timeline.Update(body)
				if reveal != nil {
					timeline.Reveal(reveal, body)
					reveal = nil
				}
			}
		}

		bx := w - 24 - 14
		showAll.Bounds = rl.NewRectangle(bx-showAll.Bounds.Width, body.Y+(ui.TimelineBar-34)/2, showAll.Bounds.Width, 34)
		editAlbum.Bounds = rl.NewRectangle(bx-editAlbum.Bounds.Width, body.Y+(ui.TimelineBar-34)/2, editAlbum.Bounds.Width, 34)
		backAlbums.Bounds = rl.NewRectangle(editAlbum.Bounds.X-8-backAlbums.Bounds.Width, editAlbum.Bounds.Y, backAlbums.Bounds.Width, 34)
		if !modal && !selecting {
			switch {
			case searching:
				showAll.Update()
			case albumOpen:
				editAlbum.Update()
				backAlbums.Update()
			}
		}

		buttonHover := !selecting && ((searching && showAll.Hovered()) || (albumOpen && (editAlbum.Hovered() || backAlbums.Hovered())))
		switch {
		case modal && editor.InputHovered():
			rl.SetMouseCursor(rl.MouseCursorIBeam)
		case modal:
			if picker.Hovered() || editor.Hovered() {
				rl.SetMouseCursor(rl.MouseCursorPointingHand)
			} else {
				rl.SetMouseCursor(rl.MouseCursorDefault)
			}
		case input.Hovered():
			rl.SetMouseCursor(rl.MouseCursorIBeam)
		case nav.Hovered() || buttonHover || (gridMode && grid.Hovered()) || (!gridMode && (timeline.ScrubHovered() || timeline.TileHovered())) || (selecting && selBar.Hovered()) || (nav.View == ui.ViewPeople && people.Hovered()):
			rl.SetMouseCursor(rl.MouseCursorPointingHand)
		default:
			rl.SetMouseCursor(rl.MouseCursorDefault)
		}

		rl.BeginDrawing()
		backdrop.Begin()
		rl.ClearBackground(theme.Background)
		switch {
		case gridMode:
			grid.Draw(body)
		default:
			if topH > 0 {
				people.Draw(top)
			}
			timeline.Draw(body)
			switch {
			case selecting:
				selBar.Draw(barR)
			case searching:
				showAll.Draw()
			case albumOpen:
				editAlbum.Draw()
				backAlbums.Draw()
			}
		}
		backdrop.End()
		nav.Draw(backdrop)
		toast.Draw(theme, nav.Bounds())
		picker.Draw(screen)
		editor.Draw(screen)
		rl.EndDrawing()
	}
}
