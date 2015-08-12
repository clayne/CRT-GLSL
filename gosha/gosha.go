package main

import (
	"fmt"
	//"os"; "log"
	_"math"
	"image"
	_ "image/png"
    "sync"
    "runtime"
	"azul3d.org/gfx.v1"
	"azul3d.org/gfx/window.v2"
	"azul3d.org/keyboard.v1"
	"azul3d.org/lmath.v1"
    "azul3d.org/clock.v1"
)

var glslVert = []byte(`
#version 120

attribute vec3 Vertex;
attribute vec2 TexCoord0;

uniform mat4 MVP;

void main()
{
        gl_TexCoord[0].st = TexCoord0;
        gl_Position = MVP * vec4(Vertex, 1.0);

        //gl_TexCoord[0]  = gl_TextureMatrix[0] * gl_MultiTexCoord0;
        //gl_Position     = ftransform();
}
`)

func createShaders(manager ShaderManager, size image.Point) []*gfx.Shader {
    shaders := make([]*gfx.Shader, len(manager.Current().FragSrc))
    for i, fragSource := range manager.Current().FragSrc {
        shaders[i] = gfx.NewShader(fmt.Sprintf("%s-pass%d", manager.Current().Name, i+1))
        shaders[i].GLSLVert = glslVert
        shaders[i].GLSLFrag = []byte(fragSource)
        shaders[i].Inputs["color_texture_sz"] = gfx.Vec3{float32(size.X), float32(size.Y), 0.0}
        for uniform,value := range *manager.Current().Defaults {
            shaders[i].Inputs[uniform] = value
        }        
    }    
    fmt.Println("Created shaderset ", func() (res []string) { 
            for _,s := range shaders {
                res = append(res, s.Name)
            }
            return
        }())
    return shaders
}

func updateWindowTitle(w window.Window, descr *ShaderDescriptor) {
    props := w.Props()
    props.SetTitle(descr.Name + " {FPS}")
    w.Request(props)
}

func updateWindowSize(w window.Window, size image.Point) (sizeChanged bool) {
    props := w.Props()
    width, height := props.Size()
    if width == size.X && height == size.Y {
        return false
    }
    props.SetSize(size.X, size.Y)
    w.Request(props)
    return true
}

type CommandCode int
type Command struct {
    Code CommandCode
}

const CmdLoadShader     CommandCode = 0
const CmdNextShader     CommandCode = 1
const CmdQuit           CommandCode = 2
const CmdResize         CommandCode = 32
const CmdLoadImage      CommandCode = 33
const CmdImageLoaded    CommandCode = 34

type ShaderTargetPair struct {
    Shader *gfx.Shader
    Canvas gfx.Canvas
    MpassTex *gfx.Texture
}

func handleEvents(events chan window.Event, commands chan Command) {
    var modifier keyboard.Key
    for e := range events {
        //fmt.Println(e)
        switch ev := e.(type) {
        case window.FramebufferResized:
            commands <- Command{Code: CmdResize}
        case keyboard.StateEvent:
            if ev.Key == keyboard.LeftSuper {
                if ev.State == keyboard.Down {
                    modifier = ev.Key
                } else {
                    modifier = 0
                }
            }
            if ev.State == keyboard.Down {
                if ev.Key == keyboard.Escape ||
                    (ev.Key == keyboard.Q && modifier == keyboard.LeftSuper) {
                    commands <- Command{Code: CmdQuit}
                }
                switch ev.Key {
                    case keyboard.N:
                        commands <- Command{Code: CmdNextShader}
                    case keyboard.M:
                        commands <- Command{Code: CmdLoadImage}
                }
            }
        }
    }
}

/*
    (0,0) (1,0)
       *--*
       | /|
       |/ |
       *--*
    (0,1) (1,1)
start here
*/    
func createCard() (card *gfx.Object) {    
    cardMesh := gfx.NewMesh()
    cardMesh.Vertices = []gfx.Vec3 {
        // Bottom-left triangle.
        {0, 0, 1},
        {1, 0, 0},
        {1, 0, 1},
        // Top-right triangle.
        {0, 0,  1},
        {0, 0,  0},
        {1, 0,  0},
    }
    cardMesh.TexCoords = []gfx.TexCoordSet{
        {
            Slice: []gfx.TexCoord{
                {0, 0},
                {1, 1},
                {1, 0},

                {0, 0},
                {0, 1},
                {1, 1},
            },
        },
    }
    card = gfx.NewObject()
    card.AlphaMode = gfx.AlphaToCoverage
    card.Meshes = []*gfx.Mesh{cardMesh}
    return card
}

func createTexture(bitmap image.Image) *gfx.Texture {
    tex := gfx.NewTexture()
    tex.Source = bitmap
    tex.MinFilter = gfx.LinearMipmapLinear
    tex.MagFilter = gfx.Linear
    tex.Format = gfx.DXT1RGBA
    tex.WrapU, tex.WrapV = gfx.Clamp, gfx.Clamp
    return tex
}

func createMpassBuffers(r gfx.Renderer, bounds image.Rectangle) (rttTexture []*gfx.Texture, rttCanvas[]gfx.Canvas) {
    // create 2 textures for mpass ping ponging
    rttTexture = make([]*gfx.Texture, 2)
    rttCanvas = make([]gfx.Canvas, 2)
    for i, _ := range rttTexture {
        rttTexture[i] = createTexture(nil)
        rttTexture[i].Bounds = bounds

        // Choose a render to texture format.
        cfg := r.GPUInfo().RTTFormats.ChooseConfig(gfx.Precision{
            RedBits: 8, GreenBits: 8, BlueBits: 8, AlphaBits: 8,
        }, true)
        cfg.Color = rttTexture[i]
        cfg.Bounds = bounds
        rttCanvas[i] = r.RenderToTexture(cfg)
    }
    return
}

func gfxLoop(w window.Window, r gfx.Renderer) {
    shaderManager := NewShaderManager()
    imageLoader := NewImageLoader("../glsl/images")

    var shaders []*gfx.Shader
    var img image.Image
    var sourceTexture *gfx.Texture
    var rttTexture []*gfx.Texture
    var rttCanvas []gfx.Canvas
    
    screenCamera := gfx.NewCamera()
    screenCamera.SetPos(lmath.Vec3{0, -2, 0})
    card := createCard()

    // Create a channel of events.
    events := make(chan window.Event, 256)
    commands := make(chan Command, 256)
    // Have the window notify our channel whenever events occur.
    w.Notify(events, window.FramebufferResizedEvents|window.KeyboardTypedEvents|window.KeyboardStateEvents)

    lock := sync.RWMutex{}
    couples := []ShaderTargetPair{}

    running := true
    go handleEvents(events, commands)
    go func(commands chan Command) {
        for command := range commands {
            switch command.Code {
            case CmdQuit:
                running = false
            case CmdLoadImage:
                img = imageLoader.Next()
                if img != nil {
                    commands <- Command{Code: CmdImageLoaded}
                }
            case CmdImageLoaded:
                lock.Lock()
                sourceTexture = createTexture(img)
                rttTexture, rttCanvas = createMpassBuffers(r, img.Bounds())
                lock.Unlock()
                if !updateWindowSize(w, img.Bounds().Max) {
                    // resize will create mpass buffers, then request shader load
                    commands <- Command{Code: CmdResize}     
                }
            case CmdResize:
                commands <- Command{Code: CmdLoadShader}
            case CmdNextShader:
                shaderManager.LoadNext()
                commands <- Command{Code: CmdLoadShader}
            case CmdLoadShader:
                lock.Lock()

                shaders = createShaders(shaderManager, img.Bounds().Max)
                // init render targets: mpass canvases and main renderer
                couples = make([]ShaderTargetPair, len(shaders) - 1, len(shaders))
                for i := range couples {
                    couples[i] = ShaderTargetPair {
                            Shader: shaders[i],
                            Canvas: rttCanvas[i % 2],
                            MpassTex: rttTexture[(i + 1) % 2],
                        }
                }
                couples = append(couples, ShaderTargetPair {
                        Shader: shaders[len(shaders) - 1],
                        Canvas: r,
                        MpassTex: rttTexture[len(shaders) % 2],
                    })

                lock.Unlock()
                go updateWindowTitle(w, shaderManager.Current())
            }
        }
    }(commands)

    // Start with loading an image
    commands <- Command{Code: CmdLoadImage}

    clock := clock.New()
    clock.SetMaxFrameRate(70)

    for running {
        for card == nil {
            runtime.Gosched()
        }

        lock.Lock()
        for _, couple := range couples {
            couple.Canvas.Clear(image.Rect(0, 0, 0, 0), gfx.Color{1, 0, 1, 1})
            couple.Canvas.ClearDepth(image.Rect(0, 0, 0, 0), 1.0)

            b := couple.Canvas.Bounds() 
            screenCamera.SetOrtho(b, 0.001, 1000.0)
            card.SetPos(lmath.Vec3{0, 0, 0})

            // Scale the card to fit the window.
            s := float64(b.Dy())
            ratio := float64(b.Dx()) / float64(b.Dy());
            card.SetScale(lmath.Vec3{s * ratio, 1.0, s})

            card.Shader = couple.Shader
            // Texture0 is source, Texture1 is mpass source
            card.Textures = []*gfx.Texture{sourceTexture, couple.MpassTex}
            couple.Canvas.Draw(image.Rect(0, 0, 0, 0), card, screenCamera)
            couple.Canvas.Render()
        }

        lock.Unlock()
        clock.Tick()
        runtime.Gosched()
    }    
    w.Close()
}

func main() {
	window.Run(gfxLoop, nil)
}