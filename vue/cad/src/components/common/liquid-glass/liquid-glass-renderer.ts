/**
 * 精简版 WebGL2 多通道渲染器，移植自 iyinchao/liquid-glass-studio 的 GLUtils.ts（MIT）。
 * 仅保留 WebGL2 后端：ShaderProgram / FrameBuffer / RenderPass / MultiPassRenderer。
 */

type GL = WebGL2RenderingContext

export interface RenderPassConfig {
  name: string
  vertex: string
  fragment: string
  inputs?: Record<string, string>
  outputToScreen?: boolean
}

interface UniformInfo {
  location: WebGLUniformLocation
  type: number
  isArray: boolean
}

class ShaderProgram {
  private readonly gl: GL
  private readonly program: WebGLProgram
  private readonly uniforms = new Map<string, UniformInfo>()
  private readonly attributes = new Map<string, number>()

  constructor(gl: GL, vertex: string, fragment: string) {
    this.gl = gl
    this.program = this.createProgram(vertex, fragment)
    this.detectAttributes()
    this.detectUniforms()
  }

  private compile(type: number, source: string): WebGLShader {
    const gl = this.gl
    const shader = gl.createShader(type)
    if (!shader) throw new Error('液态玻璃：创建着色器失败')
    gl.shaderSource(shader, source)
    gl.compileShader(shader)
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      const info = gl.getShaderInfoLog(shader)
      gl.deleteShader(shader)
      throw new Error(`液态玻璃：着色器编译失败 ${info ?? ''}`)
    }
    return shader
  }

  private createProgram(vertex: string, fragment: string): WebGLProgram {
    const gl = this.gl
    const program = gl.createProgram()
    if (!program) throw new Error('液态玻璃：创建程序失败')
    const vs = this.compile(gl.VERTEX_SHADER, vertex)
    const fs = this.compile(gl.FRAGMENT_SHADER, fragment)
    gl.attachShader(program, vs)
    gl.attachShader(program, fs)
    gl.linkProgram(program)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      const info = gl.getProgramInfoLog(program)
      gl.deleteProgram(program)
      throw new Error(`液态玻璃：程序链接失败 ${info ?? ''}`)
    }
    gl.deleteShader(vs)
    gl.deleteShader(fs)
    return program
  }

  private detectAttributes(): void {
    const gl = this.gl
    const count = gl.getProgramParameter(this.program, gl.ACTIVE_ATTRIBUTES) as number
    for (let i = 0; i < count; i++) {
      const info = gl.getActiveAttrib(this.program, i)
      if (!info) continue
      this.attributes.set(info.name, gl.getAttribLocation(this.program, info.name))
    }
  }

  private detectUniforms(): void {
    const gl = this.gl
    const count = gl.getProgramParameter(this.program, gl.ACTIVE_UNIFORMS) as number
    for (let i = 0; i < count; i++) {
      const info = gl.getActiveUniform(this.program, i)
      if (!info) continue
      const name = info.name.replace(/\[0\]$/, '')
      const location = gl.getUniformLocation(this.program, info.name)
      if (!location) continue
      this.uniforms.set(name, {
        location,
        type: info.type,
        isArray: info.size > 1 || /\[0\]$/.test(info.name),
      })
    }
  }

  use(): void {
    this.gl.useProgram(this.program)
  }

  getAttributeLocation(name: string): number {
    return this.attributes.get(name) ?? -1
  }

  setUniform(name: string, value: number | number[] | Float32Array): void {
    const gl = this.gl
    const uniform = this.uniforms.get(name)
    if (!uniform) return
    const { location, type } = uniform
    const array = (v: number | number[] | Float32Array) => (v instanceof Float32Array ? v : Float32Array.from(v as number[]))
    switch (type) {
      case gl.FLOAT:
        if (Array.isArray(value) || value instanceof Float32Array) gl.uniform1fv(location, array(value))
        else gl.uniform1f(location, value)
        break
      case gl.FLOAT_VEC2:
        gl.uniform2fv(location, array(value))
        break
      case gl.FLOAT_VEC3:
        gl.uniform3fv(location, array(value))
        break
      case gl.FLOAT_VEC4:
        gl.uniform4fv(location, array(value))
        break
      case gl.INT:
      case gl.BOOL:
      case gl.SAMPLER_2D:
        gl.uniform1i(location, value as number)
        break
      default:
        break
    }
  }

  dispose(): void {
    this.gl.deleteProgram(this.program)
    this.uniforms.clear()
    this.attributes.clear()
  }
}

class FrameBuffer {
  private readonly gl: GL
  private readonly fbo: WebGLFramebuffer
  private readonly texture: WebGLTexture
  private width: number
  private height: number

  constructor(gl: GL, width: number, height: number) {
    this.gl = gl
    this.width = width
    this.height = height
    const fbo = gl.createFramebuffer()
    const texture = gl.createTexture()
    if (!fbo || !texture) throw new Error('液态玻璃：创建帧缓冲失败')
    this.fbo = fbo
    this.texture = texture
    this.allocate()
    gl.bindFramebuffer(gl.FRAMEBUFFER, null)
    gl.bindTexture(gl.TEXTURE_2D, null)
  }

  private allocate(): void {
    const gl = this.gl
    gl.bindFramebuffer(gl.FRAMEBUFFER, this.fbo)
    gl.bindTexture(gl.TEXTURE_2D, this.texture)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA16F, this.width, this.height, 0, gl.RGBA, gl.FLOAT, null)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, this.texture, 0)
    const status = gl.checkFramebufferStatus(gl.FRAMEBUFFER)
    if (status !== gl.FRAMEBUFFER_COMPLETE) throw new Error(`液态玻璃：帧缓冲不完整 ${status}`)
  }

  bind(): void {
    this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, this.fbo)
  }

  unbind(): void {
    this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null)
  }

  getTexture(): WebGLTexture {
    return this.texture
  }

  resize(width: number, height: number): void {
    if (width === this.width && height === this.height) return
    this.width = width
    this.height = height
    this.allocate()
    this.unbind()
    this.gl.bindTexture(this.gl.TEXTURE_2D, null)
  }

  dispose(): void {
    this.gl.deleteFramebuffer(this.fbo)
    this.gl.deleteTexture(this.texture)
  }
}

class RenderPass {
  readonly config: RenderPassConfig
  private readonly gl: GL
  private readonly program: ShaderProgram
  private readonly frameBuffer: FrameBuffer | null
  private readonly vao: WebGLVertexArrayObject
  private readonly vertexBuffer: WebGLBuffer

  constructor(gl: GL, config: RenderPassConfig) {
    this.gl = gl
    this.config = config
    this.program = new ShaderProgram(gl, config.vertex, config.fragment)
    this.frameBuffer = config.outputToScreen ? null : new FrameBuffer(gl, gl.canvas.width, gl.canvas.height)
    const vao = gl.createVertexArray()
    const buffer = gl.createBuffer()
    if (!vao || !buffer) throw new Error('液态玻璃：创建顶点缓冲失败')
    this.vao = vao
    this.vertexBuffer = buffer
    gl.bindVertexArray(vao)
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
    const loc = this.program.getAttributeLocation('a_position')
    gl.enableVertexAttribArray(loc)
    gl.vertexAttribPointer(loc, 2, gl.FLOAT, false, 0, 0)
    gl.bindVertexArray(null)
    gl.bindBuffer(gl.ARRAY_BUFFER, null)
  }

  render(uniforms: Record<string, number | number[] | Float32Array | WebGLTexture>): void {
    const gl = this.gl
    if (this.frameBuffer) this.frameBuffer.bind()
    else gl.bindFramebuffer(gl.FRAMEBUFFER, null)

    this.program.use()
    let textureUnit = 0
    for (const [name, value] of Object.entries(uniforms)) {
      if (value instanceof WebGLTexture) {
        gl.activeTexture(gl.TEXTURE0 + textureUnit)
        gl.bindTexture(gl.TEXTURE_2D, value)
        this.program.setUniform(name, textureUnit)
        textureUnit += 1
      } else {
        this.program.setUniform(name, value)
      }
    }
    gl.bindVertexArray(this.vao)
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
    gl.bindVertexArray(null)
    if (this.frameBuffer) this.frameBuffer.unbind()
  }

  getOutputTexture(): WebGLTexture | null {
    return this.frameBuffer?.getTexture() ?? null
  }

  resize(width: number, height: number): void {
    this.frameBuffer?.resize(width, height)
  }

  dispose(): void {
    this.frameBuffer?.dispose()
    this.program.dispose()
    this.gl.deleteBuffer(this.vertexBuffer)
    this.gl.deleteVertexArray(this.vao)
  }
}

export type UniformMap = Record<string, number | number[] | Float32Array | WebGLTexture>

export class LiquidGlassRenderer {
  private readonly gl: GL
  private readonly passes: RenderPass[] = []
  private readonly passMap = new Map<string, RenderPass>()
  private readonly globalUniforms: UniformMap = {}

  constructor(canvas: HTMLCanvasElement, configs: RenderPassConfig[]) {
    const gl = canvas.getContext('webgl2', { alpha: true, antialias: false, premultipliedAlpha: false })
    if (!gl) throw new Error('液态玻璃：当前环境不支持 WebGL2')
    if (!gl.getExtension('EXT_color_buffer_float')) throw new Error('液态玻璃：缺少 EXT_color_buffer_float 扩展')
    this.gl = gl
    for (const config of configs) {
      const pass = new RenderPass(gl, config)
      this.passes.push(pass)
      this.passMap.set(config.name, pass)
    }
  }

  setUniform(name: string, value: number | number[] | Float32Array): void {
    this.globalUniforms[name] = value
  }

  setUniforms(uniforms: Record<string, number | number[] | Float32Array>): void {
    Object.assign(this.globalUniforms, uniforms)
  }

  resize(width: number, height: number): void {
    this.gl.canvas.width = width
    this.gl.canvas.height = height
    // 修改 canvas 尺寸会重置绘图缓冲，viewport 不会自动跟随，必须显式设置。
    this.gl.viewport(0, 0, width, height)
    for (const pass of this.passes) pass.resize(width, height)
  }

  render(passUniforms: Record<string, UniformMap>): void {
    const gl = this.gl
    gl.viewport(0, 0, gl.drawingBufferWidth, gl.drawingBufferHeight)
    for (const pass of this.passes) {
      const uniforms: UniformMap = { ...this.globalUniforms, ...(passUniforms[pass.config.name] ?? {}) }
      if (pass.config.inputs) {
        for (const [uniformName, fromPass] of Object.entries(pass.config.inputs)) {
          const texture = this.passMap.get(fromPass)?.getOutputTexture()
          if (texture) uniforms[uniformName] = texture
        }
      }
      pass.render(uniforms)
    }
  }

  createTexture(): WebGLTexture {
    const gl = this.gl
    const texture = gl.createTexture()
    if (!texture) throw new Error('液态玻璃：创建纹理失败')
    gl.bindTexture(gl.TEXTURE_2D, texture)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array([255, 255, 255, 255]))
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    gl.bindTexture(gl.TEXTURE_2D, null)
    return texture
  }

  dispose(): void {
    for (const pass of this.passes) pass.dispose()
    this.passes.length = 0
    this.passMap.clear()
    this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null)
    this.gl.bindTexture(this.gl.TEXTURE_2D, null)
  }
}

export function computeGaussianKernelByRadius(radius: number): number[] {
  const r = Math.max(1, Math.min(200, Math.round(radius)))
  const sigma = r / 3
  const weights: number[] = []
  let sum = 0
  for (let i = 0; i <= r; i++) {
    const w = Math.exp(-(i * i) / (2 * sigma * sigma))
    weights.push(w)
    sum += i === 0 ? w : 2 * w
  }
  return weights.map((w) => w / sum)
}
