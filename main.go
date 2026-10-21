package main

import (
	"fmt"
	"io"
	"log"
	"os"
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
			parts = append(parts, "procurando fotos…")
		} else {
			parts = append(parts, fmt.Sprintf("lendo %d/%d", st.Cataloged, st.Discovered))
		}
	}
	switch st.State {
	case search.StateLoading:
		parts = append(parts, "carregando modelos…")
	case search.StateIndexing:
		parts = append(parts, fmt.Sprintf("indexando %d/%d", st.Indexed, max(st.Total, st.Cataloged)))
		parts = append(parts, fmt.Sprintf("%d rostos", st.Faces))
		if st.Remaining > 0 {
			parts = append(parts, formatRemaining(st.Remaining))
		}
	case search.StateReady:
		if st.FaceErr != nil {
			parts = append(parts, "rostos indisponíveis: "+st.FaceErr.Error())
		}
	}
	return strings.Join(parts, "  •  ")
}

func formatRemaining(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "menos de 1 min restante"
	case d < time.Hour:
		return fmt.Sprintf("~%d min restantes", int(d.Round(time.Minute).Minutes()))
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("~%dh%02d restantes", h, m)
	}
}

func searchPlaceholder(st search.Status, empty bool) string {
	switch {
	case empty:
		return "Coloque fotos em ./storage"
	case st.State == search.StateUnavailable && st.Err != nil:
		return "Busca indisponível: " + st.Err.Error()
	case st.State == search.StateLoading:
		return "Carregando modelo…"
	default:
		return "Pesquisar nas suas fotos"
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

	finder := search.Start(dirs.Storage, search.NewPaths(dirs.Models, dirs.Runtime, dirs.Database()))
	defer finder.Close()
	lib := finder.Library()
	libVersion := finder.LibraryVersion()
	var libRefreshed time.Time
	var loggedErr, loggedFaceErr, loggedCatalog bool

	albums := ui.NewAlbumStrip(theme, thumbs, lib.Albums)
	albums.Empty = "Nenhum álbum. Crie subpastas em ./storage"

	timeline := ui.NewTimeline(theme, thumbs)
	allTitle := "Fotos"
	timeline.SetPhotos(allTitle, lib.Photos)

	input := ui.NewTextInput(theme, "")
	searching := false

	people := ui.NewPeopleStrip(theme)
	defer people.Unload()
	peopleVersion := -1

	showAllPhotos := func() {
		albums.Selected = nil
		people.Selected = 0
		searching = false
		input.Text = ""
		timeline.SetPhotos(allTitle, lib.Photos)
	}

	input.OnSubmit = func(q string) {
		if strings.TrimSpace(q) == "" {
			showAllPhotos()
			return
		}
		if finder != nil {
			finder.Query(q)
		}
	}

	showAll := ui.NewButton(0, 0, 120, 34, "Ver todas", showAllPhotos)
	showAll.Font = &theme.Medium
	showAll.Style = theme.ButtonStyle()

	for !rl.WindowShouldClose() {
		w := float32(rl.GetScreenWidth())
		h := float32(rl.GetScreenHeight())
		topH := float32(ui.AlbumsHeight)
		if len(people.People) > 0 {
			topH += ui.PeopleHeight
		}
		top := rl.NewRectangle(0, 0, w, ui.AlbumsHeight)
		peopleArea := rl.NewRectangle(0, ui.AlbumsHeight, w, topH-ui.AlbumsHeight)
		bottom := rl.NewRectangle(0, topH+1, w, h-topH-1)

		thumbs.BeginFrame()

		status := search.Status{State: search.StateUnavailable}
		if finder != nil {
			status = finder.Status()
			if status.Err != nil && !loggedErr {
				log.Printf("busca: %v", status.Err)
				loggedErr = true
			}
			if status.FaceErr != nil && !loggedFaceErr {
				log.Printf("rostos: %v", status.FaceErr)
				loggedFaceErr = true
			}
			if r, ok := finder.Poll(); ok {
				albums.Selected = nil
				people.Selected = 0
				searching = true
				if r.Err != nil {
					timeline.SetResults("“"+r.Query+"”", "Erro: "+r.Err.Error(), nil)
				} else {
					timeline.SetResults("“"+r.Query+"”", fmt.Sprintf("Mais relevantes  ·  %d ms", r.Took.Milliseconds()), r.Photos)
				}
			}
			if v := finder.PeopleVersion(); v != peopleVersion {
				peopleVersion = v
				people.SetPeople(finder.People())
			}
		}
		if v := finder.LibraryVersion(); v != libVersion && time.Since(libRefreshed) > 400*time.Millisecond {
			libVersion = v
			libRefreshed = time.Now()
			lib = finder.Library()
			albums.SetAlbums(lib.Albums)
			switch {
			case searching:
			case albums.Selected != nil:
				timeline.Refresh(albums.Selected.Photos)
			case people.Selected != 0:
				if p, ok := people.SelectedPerson(); ok {
					timeline.Refresh(finder.PhotosOf(p))
				}
			default:
				timeline.Refresh(lib.Photos)
			}
		}
		if !status.Reading && !loggedCatalog {
			log.Printf("%d fotos em %s", status.Cataloged, dirs.Relative(dirs.Storage))
			loggedCatalog = true
		}

		empty := !status.Reading && status.Cataloged == 0
		timeline.EmptyText = "Coloque fotos em ./storage"
		if status.Reading {
			timeline.EmptyText = "Lendo ./storage…"
		}
		timeline.Status = searchStatus(status)
		input.Placeholder = searchPlaceholder(status, empty)

		if len(people.People) > 0 && people.Update(peopleArea) {
			searching = false
			input.Text = ""
			albums.Selected = nil
			if p, ok := people.SelectedPerson(); ok {
				name := p.Name
				if name == "" {
					name = fmt.Sprintf("Pessoa %d", p.ID)
				}
				timeline.SetPhotos(name, finder.PhotosOf(p))
			} else {
				timeline.SetPhotos(allTitle, lib.Photos)
			}
		}

		if albums.Update(top) {
			people.Selected = 0
			searching = false
			input.Text = ""
			if albums.Selected != nil {
				timeline.SetPhotos(albums.Selected.Name, albums.Selected.Photos)
			} else {
				timeline.SetPhotos(allTitle, lib.Photos)
			}
		}

		barY := bottom.Y + (ui.TimelineBar-36)/2
		inputW := min(360, w*0.35)
		input.Bounds = rl.NewRectangle(w-24-inputW, barY, inputW, 36)
		input.Update()

		timeline.KeysEnabled = !input.Focused
		timeline.Update(bottom)

		filtered := albums.Selected != nil || people.Selected != 0 || searching
		showAll.Bounds = rl.NewRectangle(input.Bounds.X-12-showAll.Bounds.Width, bottom.Y+(ui.TimelineBar-34)/2, showAll.Bounds.Width, 34)
		if filtered {
			showAll.Update()
		}

		switch {
		case input.Hovered():
			rl.SetMouseCursor(rl.MouseCursorIBeam)
		case albums.Hovered() || people.Hovered() || (filtered && showAll.Hovered()):
			rl.SetMouseCursor(rl.MouseCursorPointingHand)
		default:
			rl.SetMouseCursor(rl.MouseCursorDefault)
		}

		rl.BeginDrawing()
		rl.ClearBackground(theme.Background)
		albums.Draw(top)
		if len(people.People) > 0 {
			people.Draw(peopleArea)
		}
		rl.DrawRectangle(0, int32(topH), int32(w), 1, theme.Divider)
		timeline.Draw(bottom)
		input.Draw()
		if filtered {
			showAll.Draw()
		}
		rl.EndDrawing()
	}
}
