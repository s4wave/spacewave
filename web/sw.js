// Note: we use this "proxy" file to place sw.js at /.
// This way the scope will control pages at / instead of /bldr/.
if (self && self.importScripts) {
  self.importScripts('./bldr/service-worker.js')
}
