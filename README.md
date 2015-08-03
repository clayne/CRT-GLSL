# Software PAL modulation/demodulation experiments

The idea is to reproduce realistic composite-like artifacting by applying PAL modulation
and demodulation, like the analog signal. This repository is highly experimental and probably contains no
user-serviceable code.

The project contains 2 main parts: purely software simulation and GLSL shader implementation. The GLSL 
part consists of a Python host which can be used to experiment with any fragment shaders, and the shaders themselves.

## Shaders
Currently shaders have 3 different implementations that aim to reproduce the same model.
### mpass
3-stage processing:
 1. modulate source RGB signal to B&W image
 2. demodulate U and V, pass (Modulated, U, V) in (R,G,B) channels
 3. lowpass filter UV at baseband, remodulate again and subtract from Modulated to recover Luma. Convert back to RGB
 
### oversampling
SDLMESS uses very small textures for multipass shading, which makes them 
not very practical for purposes of storing intermediate values. This method is an attempt to pack 2x more
intermediate values in unused texture channels. It differs from mpass in that it passes U,V,U,V in (RGBA), thus
packing 2x more bandwidth in same amount of pixels. 

### singlepass
I was afraid that this method would be very slow because a lot of things are calculated over and over again
for the purpose of filtering. But it seems to be doing fine even on slower GPUs. This is the best method.

## Requirements:
Software-only model: Python 2.7, PyPNG.
GLSL model: Python 2.7, PyGame, PyOpenGL.

## SDLMESS/SDLMAME compatibility
This toy is designed with SDLMESS compatibility in mind, so shaders designed with it can be used with 
SDLMESS almost without modifications. 

## Acknowledgements
I cannibalized Ian Mallett's GLSL Game of Life code as initial PyGame/PyOpenGL boilerplate code.
His work can be found here: http://www.geometrian.com/programming/projects/index.php?project=Game%20of%20Life



