const h=["var(--blue)","var(--green)","var(--amber)","hsl(280, 55%, 65%)","var(--red)","hsl(175, 50%, 50%)","hsl(330, 55%, 60%)","hsl(200, 60%, 55%)"];function a(e){let r=0;for(let t=0;t<e.length;t++)r=(r<<5)-r+e.charCodeAt(t),r|=0;return Math.abs(r)}function l(e){return h[a(e)%h.length]}export{l as s};
//# sourceMappingURL=clusterColors-DkNK0EU6.js.map
