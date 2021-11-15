package gol

import (
	"fmt"
	"strconv"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

type CaculateStruct struct {

}

type RpcRequest struct {
	World *World
	Turn  int
}

type RpcResponse struct {
	RpcStatus int
	World     *World
	Turn      int
}

func (this *CaculateStruct) Caculate(req RpcRequest, resp *RpcResponse) error {
	fmt.Printf("start to Caculate\n")
	//req.World.DebugLog(req.Turn)
	for i := 0; i < req.Turn; i++ {
		req.World.NextStep()
	}
	//req.World.DebugLog(req.Turn)

	resp.World = req.World
	resp.RpcStatus = 0
	resp.Turn = req.Turn
	return nil
}

////////-------单机--------///////
// 整个世界
type World struct {
	Grid1   *Grid // 细胞生命状态
	Grid2   *Grid // 临时存储前一轮细胞声明状态
	Width   int   // 宽度
	Height  int   // 高度
	Threads int   // 线程数
}

// 单元格
type Grid struct {
	Status  [][]bool // true代表细胞活着，false代表细胞死亡
	Width   int      // 宽度
	Height  int      // 高度
	Threads int      // 线程数
}

// 初始化grid, 申请内存空间
func NewGrid(w, h, t int) *Grid {
	g := Grid{}
	g.Width = w
	g.Height = h
	g.Threads = t
	g.Status = make([][]bool, h)
	for i := 0; i < h; i++ {
		g.Status[i] = make([]bool, w)
	}

	return &g
}

// 初始化world
func NewWorld(w, h, t int) *World {
	world := World{}
	world.Height = h
	world.Width = w
	world.Threads = t
	world.Grid1 = NewGrid(w, h, t)
	world.Grid2 = NewGrid(w, h, t)
	return &world
}

// 设置x，y细胞的状态
func (g *Grid) Set(x, y int, status bool) {
	g.Status[x][y] = status
}

// 计算细胞坐标位置，因为可能越界
func (g *Grid) Alive(x, y int) bool {
	x = (x + g.Width) % g.Width
	y = (y + g.Height) % g.Height
	//fmt.Printf("%v %v %v %v\n", x, y, len(g.status), len(g.status[0]))
	return g.Status[x][y]
}

// 下一轮(x,y)坐标细胞的状态计算
func (g *Grid) NextStatus(x, y int) bool {
	alive := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			// i = 0, j = 0 时为本细胞状态，不计算自己的状态
			// example
			// 0 0 0
			// 0 1 1
			// 1 0 1
			// 比如上图中，中心位置1代表当前细胞，则计算出来的alive为3
			if (i != 0 || j != 0) && g.Alive(x+i, y+j) {
				alive++
			}
		}
	}

	// 大多数情况都是周围没有细胞，加速判断过程
	if alive == 0 {
		//fmt.Printf("x=%v y=%v alive=0\n", x, y)
		return false
	}
	// 按照定义，周围有3个活细胞，则一定是活状态
	// 周围有2两活细胞，自己是活细胞为活状态
	// 其余都为死状态
	if alive == 3 {
		//fmt.Printf("x=%v y=%v alive=3\n", x, y)
		return true
	}

	return alive == 2 && g.Alive(x, y)
}

// 下一轮所有的细胞状态计算, 并发逻辑在这里实现
func (w *World) NextStep() {
	wg := sync.WaitGroup{}
	//每个worker需要计算的宽度
	lenght := w.Width / w.Threads
	for i := 0; i < w.Threads; i++ {
		wg.Add(1)
		// 启动worker进行计算
		start := i * lenght
		end := start + lenght - 1
		if i == w.Threads-1 {
			end = w.Width - 1
		}
		go func(wg *sync.WaitGroup, index, startIndex, endIndex int) {
			defer wg.Done()
			for x := 0; x < w.Height; x++ {
				for y := startIndex; y <= endIndex; y++ {
					w.Grid2.Set(x, y, w.Grid1.NextStatus(x, y))
				}
			}
		}(&wg, i, start, end)
	}
	wg.Wait()

	w.Grid1, w.Grid2 = w.Grid2, w.Grid1
}

// 计算文件名称格式为 [Height]x[Width]
func (w *World) imageName1() string {
	return fmt.Sprintf("%sx%s", strconv.Itoa(w.Height), strconv.Itoa(w.Width))
}

// 计算文件名称格式为 [Height]x[Width]
func (w *World) imageName2(turn int) string {
	return fmt.Sprintf("%sx%sx%s", strconv.Itoa(w.Height), strconv.Itoa(w.Width), strconv.Itoa(turn))
}

// 从input中初始化world中的grid1
func (w *World) initCell(c distributorChannels) {
	c.ioCommand <- ioInput
	c.ioFilename <- w.imageName1()
	for i := 0; i < w.Height; i++ {
		for j := 0; j < w.Width; j++ {
			tmp := <-c.ioInput
			w.Grid1.Set(i, j, tmp == 255)
		}
	}
}

// 计算当前存活的细胞数量
func (w *World) AliveCount() int {
	count := 0
	for i := 0; i < w.Height; i++ {
		for j := 0; j < w.Width; j++ {
			if w.Grid1.Alive(i, j) {
				count++
			}
		}
	}

	return count
}

// 用于打印debug日志
func (w *World) DebugLog(turn int) {
	if w.Height > 64 || w.Width > 64 {
		return
	}
	fmt.Printf("-----------turn = %v-----------\n", turn)
	for i := 0; i < w.Height; i++ {
		for j := 0; j < w.Width; j++ {
			if w.Grid1.Alive(i, j) {
				fmt.Printf("@")
			} else {
				fmt.Printf("-")
			}
			fmt.Printf(" ")
		}
		fmt.Printf("\n")
	}
	fmt.Printf("-----------turn = %v-----------\n", turn)
	fmt.Printf("\n")
}

// 向events中发送final event
func (w *World) SendFinal(turn int, c distributorChannels) {
	final := FinalTurnComplete{}
	final.CompletedTurns = turn
	final.Alive = make([]util.Cell, 0)

	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if w.Grid1.Status[y][x] {
				final.Alive = append(final.Alive, util.Cell{X: x, Y: y})
			}
		}
	}

	c.events <- final
}

// 向events中发送alive count
func (w *World) SendAliveCount(turn int, c distributorChannels) {
	event := AliveCellsCount{}
	event.CompletedTurns = turn
	event.CellsCount = w.AliveCount()

	c.events <- event
}

// 向events发送已经完成一回合
func (w *World) SendCompleteOneTurn(turn int, c distributorChannels) {
	event := TurnComplete{}
	event.CompletedTurns = turn

	c.events <- event
}

func (w *World) WritePgm(turn int, c distributorChannels) {
	c.ioCommand <- ioOutput
	c.ioFilename <- w.imageName2(turn)
	for i := 0; i < w.Height; i++ {
		for j := 0; j < w.Width; j++ {
			if w.Grid1.Alive(i, j) {
				c.ioOutput <- 255
			} else {
				c.ioOutput <- 0
			}
		}
	}
}

func (w *World) DiffGrid(y, x int) bool {
	return w.Grid1.Status[y][x] != w.Grid2.Status[y][x]
}

func (w *World) SendCellFliped(turn int, c distributorChannels) {
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			// 如果和前一时代状态有变化，则发送事件
			if w.DiffGrid(y, x) {
				event := CellFlipped{}
				event.CompletedTurns = turn
				event.Cell = util.Cell{X: x, Y: y}
				c.events <- event
			}
		}
	}
}

func (w *World) Quit(turn int, c distributorChannels) {
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	t := time.Now()
	// TODO: Create a 2D slice to store the world.
	world := NewWorld(p.ImageWidth, p.ImageHeight, p.Threads)
	turn := 0
	world.initCell(c)

	// 发送初始位置
	world.SendCellFliped(turn, c)

	// TODO: Execute all turns of the Game of Life.
	// ticker定时器，每隔2s接收ticker.C
	ticker := time.NewTicker(2 * time.Second)
	for turn < p.Turns {
		//world.DebugLog(turn)
		select {
		case op := <-c.keyPresses:
			if op == 's' {
				// 存储pgm图片
				world.WritePgm(turn, c)
			} else if op == 'q' {
				// 存储pgm图片
				world.WritePgm(turn, c)
				goto quit
			} else if op == 'p' {
				fmt.Printf("turn=%v\n", turn)
				for {
					tmp := <-c.keyPresses
					if tmp == 'p' {
						fmt.Printf("Continuing!\n")
						break
					}
				}
			}
		case <-ticker.C:
			world.SendAliveCount(turn, c)
		default:
			turn++
			world.NextStep()
			world.SendCellFliped(turn, c)
			world.SendCompleteOneTurn(turn, c)
			//time.Sleep(time.Millisecond * 5000)
		}
	}
	//world.DebugLog(turn)

	// TODO: Report the final state using FinalTurnCompleteEvent.
	// 发送最后细胞的状态
	world.SendFinal(turn, c)

	// 存储pgm图片
	world.WritePgm(turn, c)

	fmt.Printf("time cost: %v\n", time.Since(t))
quit:
	// Make sure that the Io has finished any output before exiting.
	world.Quit(turn, c)

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
