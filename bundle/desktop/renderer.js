'use strict';

/* TODO This might not be used anywhere. */
window.onload = function() {
    console.log('executing runtime');
    require('./app/runtime.js');
    console.log('executing index');
    require('./app/index.js');
}
