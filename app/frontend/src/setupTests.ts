import '@testing-library/jest-dom/vitest'

// jsdom has no layout engine, so it never implements the SVG geometry
// methods d3 (and therefore mermaid) call unconditionally during layout —
// real mermaid rendering throws "getBBox is not a function" without these.
// Zeroed dimensions are fine for our tests, which assert on rendered DOM
// structure/text rather than pixel-accurate geometry (PLAN.md 12.2.1 S5e
// 步骤2 真 mermaid 集成测试).
if (typeof SVGElement !== 'undefined') {
  // These three methods live on SVGGraphicsElement/SVGTextContentElement in
  // lib.dom.d.ts, not the SVGElement base class TS sees here — patched via
  // `any` since this is a test-only jsdom polyfill, not typed production code.
  const proto = SVGElement.prototype as any
  if (!proto.getBBox) {
    // mermaid treats a getBBox() that reports both width and height as 0 as
    // proof the element isn't really attached, and throws "svg element not
    // in render tree" — so the stub must report a plausible non-zero size,
    // not just avoid throwing.
    proto.getBBox = function (this: SVGElement) {
      const text = this.textContent ?? ''
      const width = Math.max(text.length * 8, 1)
      return { x: 0, y: 0, width, height: 16, top: 0, right: width, bottom: 16, left: 0 }
    }
  }
  if (!proto.getComputedTextLength) {
    proto.getComputedTextLength = () => 0
  }
  if (!proto.getScreenCTM) {
    proto.getScreenCTM = () => null
  }
}
