declare module 'highlight.js/lib/languages/*' {
  import { LanguageFn } from 'highlight.js'
  const lang: LanguageFn
  export default lang
}
