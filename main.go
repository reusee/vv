package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/neovim/go-client/nvim"
)

func main() {
	// tcell
	screen, err := tcell.NewScreen()
	ce(err)
	defer screen.Fini()
	ce(screen.Init())
	screen.Clear()
	width, height := screen.Size()

	// start nvim
	args := []string{
		"--embed",
	}
	args = append(args, os.Args[1:]...)
	v, err := nvim.NewChildProcess(
		nvim.ChildProcessArgs(args...),
		nvim.ChildProcessServe(false),
	)
	ce(err)
	exit := make(chan struct{})
	go func() {
		v.Serve()
		close(exit)
	}()
	defer v.Close()

	// tcell events
	tcellEvents := make(chan tcell.Event, 128)
	go func() {
		for {
			tcellEvents <- screen.PollEvent()
		}
	}()

	// nvim events
	type NvimEvent struct {
		Name string
		Args [][]interface{}
	}
	nvimEvents := make(chan NvimEvent, 128)
	watchEvents := func(name string) {
		ce(v.RegisterHandler(name, func(args ...[]interface{}) {
			nvimEvents <- NvimEvent{
				Name: name,
				Args: args,
			}
		}))
	}
	watchEvents("redraw")

	// attach ui
	ce(v.AttachUI(width, height, map[string]interface{}{
		"rgb":          true,
		"ext_linegrid": true,
	}))

	// globals
	options := make(map[string]interface{})
	var defaultStyle tcell.Style

	type HighlightAttr struct {
		Foreground *int32
		Background *int32
		Special    *int32
		Reverse    bool
		Italic     bool
		Bold       bool
		Underline  bool
		Undercurl  bool
	}
	type HighlightDefine struct {
		ID      int
		RGBAttr HighlightAttr
	}
	highlightDefines := make(map[int]HighlightDefine)

	type GridCell struct {
		r rune
		s tcell.Style
	}
	grid := make([]GridCell, width*height)

	// main loop
	for {
		select {

		case ev := <-tcellEvents:
			// tcell events
			switch ev := ev.(type) {

			case *tcell.EventKey:
				var s string
				switch ev.Key() {
				case tcell.KeyEscape:
					s = "<esc>"
				case tcell.KeyBackspace2:
					s = "<bs>"
				default:
					s = string(ev.Rune())
				}
				if len(s) > 0 {
					if _, err := v.Input(s); err != nil {
						return
					}
				}

			case *tcell.EventMouse:
				//TODO

			case *tcell.EventResize:
				newWidth, newHeight := ev.Size()
				if newWidth != width || newHeight != height {
					width = newWidth
					height = newHeight
					if err := v.TryResizeUI(width, height); err != nil {
						return
					}
				}

			}

		case ev := <-nvimEvents:
			switch ev.Name {

			case "redraw":
				for _, update := range ev.Args {
					kind := update[0].(string)
					args := update[1:]

					unmarshal := func(target interface{}) {
						buf := new(bytes.Buffer)
						ce(json.NewEncoder(buf).Encode(args))
						var raws [][]json.RawMessage
						ce(json.NewDecoder(buf).Decode(&raws))
						v := reflect.New(reflect.TypeOf(target).Elem()).Elem()
						for _, raw := range raws {
							tupleValue := reflect.New(
								v.Type().Elem(),
							).Elem()
							for i := 0; i < tupleValue.NumField(); i++ {
								if i >= len(raw) {
									break
								}
								ce(json.NewDecoder(
									bytes.NewReader(raw[i]),
								).Decode(
									tupleValue.Field(i).Addr().Interface(),
								))
							}
							v = reflect.Append(v, tupleValue)
						}
						reflect.ValueOf(target).Elem().Set(v)
					}

					switch kind {

					case "option_set":
						var pairs []struct {
							Name  string
							Value interface{}
						}
						unmarshal(&pairs)
						for _, pair := range pairs {
							options[pair.Name] = pair.Value
						}

					case "default_colors_set":
						var colors []struct {
							FG int32
							BG int32
						}
						unmarshal(&colors)
						color := colors[len(colors)-1]
						fgColor := tcell.NewHexColor(color.FG)
						bgColor := tcell.NewHexColor(color.BG)
						defaultStyle = tcell.StyleDefault.
							Foreground(fgColor).
							Background(bgColor)
						screen.SetStyle(defaultStyle)

					case "hl_attr_define":
						var defines []HighlightDefine
						unmarshal(&defines)
						for _, define := range defines {
							highlightDefines[define.ID] = define
						}

					case "grid_resize":

					case "mode_info_set":
						var infos []struct {
							CursorStyleEnabled bool
							Infos              []struct {
								CursorShape    string `json:"cursor_shape"`
								CellPercentage int    `json:"cell_percentage"`
								BlinkWait      int    `json:"blinkwait"`
								BlinkOn        int    `json:"blinkon"`
								BlinkOff       int    `json:"blinkoff"`
								AttrID         int    `json:"attr_id"`
								AttrIDLm       int    `json:"attr_id_lm"`
								ShortName      string `json:"short_name"`
								Name           string `json:"name"`
								MouseShape     int    `json:"mouse_shape"`
							}
						}
						unmarshal(&infos)

					case "flush":
						screen.Show()

					case "grid_clear":
						screen.Clear()
						grid = make([]GridCell, width*height)

					case "mouse_on":
						screen.EnableMouse()

					case "grid_line":
						var datas []struct {
							Grid     int
							Row      int
							ColStart int
							Cells    []Cell
						}
						unmarshal(&datas)
						style := defaultStyle
						for _, data := range datas {
							x := data.ColStart
							y := data.Row
							for _, cell := range data.Cells {
								if cell.HighlightID != nil {
									style = defaultStyle
									if *cell.HighlightID != 0 {
										define := highlightDefines[*cell.HighlightID]
										attr := define.RGBAttr
										if attr.Foreground != nil {
											style = style.Foreground(tcell.NewHexColor(
												*attr.Foreground,
											))
										}
										if attr.Background != nil {
											style = style.Background(tcell.NewHexColor(
												*attr.Background,
											))
										}
										if attr.Reverse {
											style = style.Reverse(true)
										}
										if attr.Bold {
											style = style.Bold(true)
										}
										if attr.Underline {
											style = style.Underline(true)
										}
									}
								}
								n := 1
								if cell.Repeat != nil {
									n = *cell.Repeat
								}
								for i := 0; i < n; i++ {
									if cell.Char > 0 {
										screen.SetContent(
											x,
											y,
											cell.Char,
											nil,
											style,
										)
									}
									grid[y*width+x] = GridCell{
										r: cell.Char,
										s: style,
									}
									x++
								}
							}
						}

					case "grid_scroll":
						var infos []struct {
							Grid   int
							Top    int
							Bottom int
							Left   int
							Right  int
							Rows   int
							Cols   int
						}
						unmarshal(&infos)

						for _, info := range infos {

							if info.Rows > 0 {
								l := info.Right - info.Left
								for destY := info.Top; destY+info.Rows < info.Bottom; destY++ {
									destBegin := destY*width + info.Left
									destEnd := destBegin + l
									srcY := destY + info.Rows
									srcBegin := srcY*width + info.Left
									srcEnd := srcBegin + l
									copy(grid[destBegin:destEnd], grid[srcBegin:srcEnd])
									for i, cell := range grid[destBegin:destEnd] {
										screen.SetContent(
											info.Left+i,
											destY,
											cell.r,
											nil,
											cell.s,
										)
									}
								}

							} else if info.Rows < 0 {
								l := info.Right - info.Left
								info.Rows = -info.Rows
								for destY := info.Bottom - 1; destY-info.Rows >= info.Top; destY-- {
									destBegin := destY*width + info.Left
									destEnd := destBegin + l
									srcY := destY - info.Rows
									srcBegin := srcY*width + info.Left
									srcEnd := srcBegin + l
									copy(grid[destBegin:destEnd], grid[srcBegin:srcEnd])
									for i, cell := range grid[destBegin:destEnd] {
										screen.SetContent(
											info.Left+i,
											destY,
											cell.r,
											nil,
											cell.s,
										)
									}
								}
							}

						}

					case "grid_cursor_goto":
						var info []struct {
							Grid   int
							Row    int
							Column int
						}
						unmarshal(&info)
						screen.ShowCursor(
							info[0].Column,
							info[0].Row,
						)

					case "mode_change":
						var info []struct {
							Mode string
							ID   int
						}
						unmarshal(&info)

					case "busy_start", "busy_stop":

					default:
						log("%s %v\n", strings.ToUpper(kind), args)
					}

				}

			}

		case <-exit:
			return

		}
	}
}

type Cell struct {
	Char        rune
	HighlightID *int
	Repeat      *int
}

func (c *Cell) UnmarshalJSON(bs []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(bs))
	decoder.Token() // [
	if decoder.More() {
		var s string
		if err := decoder.Decode(&s); err != nil {
			return err
		}
		if len(s) > 0 {
			c.Char = []rune(s)[0]
		}
	}
	if decoder.More() {
		if err := decoder.Decode(&c.HighlightID); err != nil {
			return err
		}
	}
	if decoder.More() {
		if err := decoder.Decode(&c.Repeat); err != nil {
			return err
		}
	}
	return nil
}
