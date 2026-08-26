declare module 'view-dxf' {
  interface ViewDxfCallback {
    type?: string
    data?: unknown
  }

  const Viewer: new (
    data: any,
    parent: HTMLElement,
    width: number,
    height: number,
    font: any,
    callback?: (event: ViewDxfCallback) => void,
  ) => any

  export default Viewer
}
