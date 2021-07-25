# Whitelabel App

This is an example of a "Whitelabel" application built with Bldr.

The application developer chooses to use Snowpack as their build/dev tool.

The bldr app runtime is imported as a compiled Js package.

Snowpack is configured to produce a bundle which bldr can later minify and
deploy as a module to the target deployment network.

Bldr can also build / run the app as a desktop or mobile app.

The root of the "bldr" repository builds the "sandbox" which is a Snowpack-based
app using React and the Bldr app SDK. This serves as a Bldr app example as well.
