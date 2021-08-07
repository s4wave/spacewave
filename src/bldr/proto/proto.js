/*eslint-disable block-scoped-var, id-length, no-control-regex, no-magic-numbers, no-prototype-builtins, no-redeclare, no-shadow, no-var, sort-vars*/
import * as $protobuf from 'protobufjs/minimal'

// Common aliases
const $Reader = $protobuf.Reader,
  $Writer = $protobuf.Writer,
  $util = $protobuf.util

// Exported root namespace
const $root = $protobuf.roots['default'] || ($protobuf.roots['default'] = {})

export const ipc = ($root.ipc = (() => {
  /**
   * Namespace ipc.
   * @exports ipc
   * @namespace
   */
  const ipc = {}

  ipc.webview = (function () {
    /**
     * Namespace webview.
     * @memberof ipc
     * @namespace
     */
    const webview = {}

    webview.RuntimeToWebView = (function () {
      /**
       * Properties of a RuntimeToWebView.
       * @memberof ipc.webview
       * @interface IRuntimeToWebView
       * @property {ipc.webview.ICreateWebView|null} [createWebView] RuntimeToWebView createWebView
       * @property {ipc.webview.IQueryWebViewStatus|null} [queryWebViewStatus] RuntimeToWebView queryWebViewStatus
       */

      /**
       * Constructs a new RuntimeToWebView.
       * @memberof ipc.webview
       * @classdesc Represents a RuntimeToWebView.
       * @implements IRuntimeToWebView
       * @constructor
       * @param {ipc.webview.IRuntimeToWebView=} [properties] Properties to set
       */
      function RuntimeToWebView(properties) {
        if (properties)
          for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
            if (properties[keys[i]] != null) this[keys[i]] = properties[keys[i]]
      }

      /**
       * RuntimeToWebView createWebView.
       * @member {ipc.webview.ICreateWebView|null|undefined} createWebView
       * @memberof ipc.webview.RuntimeToWebView
       * @instance
       */
      RuntimeToWebView.prototype.createWebView = null

      /**
       * RuntimeToWebView queryWebViewStatus.
       * @member {ipc.webview.IQueryWebViewStatus|null|undefined} queryWebViewStatus
       * @memberof ipc.webview.RuntimeToWebView
       * @instance
       */
      RuntimeToWebView.prototype.queryWebViewStatus = null

      // OneOf field names bound to virtual getters and setters
      let $oneOfFields

      /**
       * RuntimeToWebView rpcMsg.
       * @member {"createWebView"|"queryWebViewStatus"|undefined} rpcMsg
       * @memberof ipc.webview.RuntimeToWebView
       * @instance
       */
      Object.defineProperty(RuntimeToWebView.prototype, 'rpcMsg', {
        get: $util.oneOfGetter(
          ($oneOfFields = ['createWebView', 'queryWebViewStatus'])
        ),
        set: $util.oneOfSetter($oneOfFields),
      })

      /**
       * Creates a new RuntimeToWebView instance using the specified properties.
       * @function create
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {ipc.webview.IRuntimeToWebView=} [properties] Properties to set
       * @returns {ipc.webview.RuntimeToWebView} RuntimeToWebView instance
       */
      RuntimeToWebView.create = function create(properties) {
        return new RuntimeToWebView(properties)
      }

      /**
       * Encodes the specified RuntimeToWebView message. Does not implicitly {@link ipc.webview.RuntimeToWebView.verify|verify} messages.
       * @function encode
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {ipc.webview.IRuntimeToWebView} message RuntimeToWebView message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      RuntimeToWebView.encode = function encode(message, writer) {
        if (!writer) writer = $Writer.create()
        if (
          message.createWebView != null &&
          Object.hasOwnProperty.call(message, 'createWebView')
        )
          $root.ipc.webview.CreateWebView.encode(
            message.createWebView,
            writer.uint32(/* id 1, wireType 2 =*/ 10).fork()
          ).ldelim()
        if (
          message.queryWebViewStatus != null &&
          Object.hasOwnProperty.call(message, 'queryWebViewStatus')
        )
          $root.ipc.webview.QueryWebViewStatus.encode(
            message.queryWebViewStatus,
            writer.uint32(/* id 2, wireType 2 =*/ 18).fork()
          ).ldelim()
        return writer
      }

      /**
       * Encodes the specified RuntimeToWebView message, length delimited. Does not implicitly {@link ipc.webview.RuntimeToWebView.verify|verify} messages.
       * @function encodeDelimited
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {ipc.webview.IRuntimeToWebView} message RuntimeToWebView message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      RuntimeToWebView.encodeDelimited = function encodeDelimited(
        message,
        writer
      ) {
        return this.encode(message, writer).ldelim()
      }

      /**
       * Decodes a RuntimeToWebView message from the specified reader or buffer.
       * @function decode
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @param {number} [length] Message length if known beforehand
       * @returns {ipc.webview.RuntimeToWebView} RuntimeToWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      RuntimeToWebView.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader)) reader = $Reader.create(reader)
        let end = length === undefined ? reader.len : reader.pos + length,
          message = new $root.ipc.webview.RuntimeToWebView()
        while (reader.pos < end) {
          let tag = reader.uint32()
          switch (tag >>> 3) {
            case 1:
              message.createWebView = $root.ipc.webview.CreateWebView.decode(
                reader,
                reader.uint32()
              )
              break
            case 2:
              message.queryWebViewStatus =
                $root.ipc.webview.QueryWebViewStatus.decode(
                  reader,
                  reader.uint32()
                )
              break
            default:
              reader.skipType(tag & 7)
              break
          }
        }
        return message
      }

      /**
       * Decodes a RuntimeToWebView message from the specified reader or buffer, length delimited.
       * @function decodeDelimited
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @returns {ipc.webview.RuntimeToWebView} RuntimeToWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      RuntimeToWebView.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader)) reader = new $Reader(reader)
        return this.decode(reader, reader.uint32())
      }

      /**
       * Verifies a RuntimeToWebView message.
       * @function verify
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {Object.<string,*>} message Plain object to verify
       * @returns {string|null} `null` if valid, otherwise the reason why it is not
       */
      RuntimeToWebView.verify = function verify(message) {
        if (typeof message !== 'object' || message === null)
          return 'object expected'
        let properties = {}
        if (
          message.createWebView != null &&
          message.hasOwnProperty('createWebView')
        ) {
          properties.rpcMsg = 1
          {
            let error = $root.ipc.webview.CreateWebView.verify(
              message.createWebView
            )
            if (error) return 'createWebView.' + error
          }
        }
        if (
          message.queryWebViewStatus != null &&
          message.hasOwnProperty('queryWebViewStatus')
        ) {
          if (properties.rpcMsg === 1) return 'rpcMsg: multiple values'
          properties.rpcMsg = 1
          {
            let error = $root.ipc.webview.QueryWebViewStatus.verify(
              message.queryWebViewStatus
            )
            if (error) return 'queryWebViewStatus.' + error
          }
        }
        return null
      }

      /**
       * Creates a RuntimeToWebView message from a plain object. Also converts values to their respective internal types.
       * @function fromObject
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {Object.<string,*>} object Plain object
       * @returns {ipc.webview.RuntimeToWebView} RuntimeToWebView
       */
      RuntimeToWebView.fromObject = function fromObject(object) {
        if (object instanceof $root.ipc.webview.RuntimeToWebView) return object
        let message = new $root.ipc.webview.RuntimeToWebView()
        if (object.createWebView != null) {
          if (typeof object.createWebView !== 'object')
            throw TypeError(
              '.ipc.webview.RuntimeToWebView.createWebView: object expected'
            )
          message.createWebView = $root.ipc.webview.CreateWebView.fromObject(
            object.createWebView
          )
        }
        if (object.queryWebViewStatus != null) {
          if (typeof object.queryWebViewStatus !== 'object')
            throw TypeError(
              '.ipc.webview.RuntimeToWebView.queryWebViewStatus: object expected'
            )
          message.queryWebViewStatus =
            $root.ipc.webview.QueryWebViewStatus.fromObject(
              object.queryWebViewStatus
            )
        }
        return message
      }

      /**
       * Creates a plain object from a RuntimeToWebView message. Also converts values to other types if specified.
       * @function toObject
       * @memberof ipc.webview.RuntimeToWebView
       * @static
       * @param {ipc.webview.RuntimeToWebView} message RuntimeToWebView
       * @param {$protobuf.IConversionOptions} [options] Conversion options
       * @returns {Object.<string,*>} Plain object
       */
      RuntimeToWebView.toObject = function toObject(message, options) {
        if (!options) options = {}
        let object = {}
        if (
          message.createWebView != null &&
          message.hasOwnProperty('createWebView')
        ) {
          object.createWebView = $root.ipc.webview.CreateWebView.toObject(
            message.createWebView,
            options
          )
          if (options.oneofs) object.rpcMsg = 'createWebView'
        }
        if (
          message.queryWebViewStatus != null &&
          message.hasOwnProperty('queryWebViewStatus')
        ) {
          object.queryWebViewStatus =
            $root.ipc.webview.QueryWebViewStatus.toObject(
              message.queryWebViewStatus,
              options
            )
          if (options.oneofs) object.rpcMsg = 'queryWebViewStatus'
        }
        return object
      }

      /**
       * Converts this RuntimeToWebView to JSON.
       * @function toJSON
       * @memberof ipc.webview.RuntimeToWebView
       * @instance
       * @returns {Object.<string,*>} JSON object
       */
      RuntimeToWebView.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions)
      }

      return RuntimeToWebView
    })()

    webview.WebViewToRuntime = (function () {
      /**
       * Properties of a WebViewToRuntime.
       * @memberof ipc.webview
       * @interface IWebViewToRuntime
       * @property {ipc.webview.IWebViewStatus|null} [webViewStatus] WebViewToRuntime webViewStatus
       */

      /**
       * Constructs a new WebViewToRuntime.
       * @memberof ipc.webview
       * @classdesc Represents a WebViewToRuntime.
       * @implements IWebViewToRuntime
       * @constructor
       * @param {ipc.webview.IWebViewToRuntime=} [properties] Properties to set
       */
      function WebViewToRuntime(properties) {
        if (properties)
          for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
            if (properties[keys[i]] != null) this[keys[i]] = properties[keys[i]]
      }

      /**
       * WebViewToRuntime webViewStatus.
       * @member {ipc.webview.IWebViewStatus|null|undefined} webViewStatus
       * @memberof ipc.webview.WebViewToRuntime
       * @instance
       */
      WebViewToRuntime.prototype.webViewStatus = null

      // OneOf field names bound to virtual getters and setters
      let $oneOfFields

      /**
       * WebViewToRuntime rpcMsg.
       * @member {"webViewStatus"|undefined} rpcMsg
       * @memberof ipc.webview.WebViewToRuntime
       * @instance
       */
      Object.defineProperty(WebViewToRuntime.prototype, 'rpcMsg', {
        get: $util.oneOfGetter(($oneOfFields = ['webViewStatus'])),
        set: $util.oneOfSetter($oneOfFields),
      })

      /**
       * Creates a new WebViewToRuntime instance using the specified properties.
       * @function create
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {ipc.webview.IWebViewToRuntime=} [properties] Properties to set
       * @returns {ipc.webview.WebViewToRuntime} WebViewToRuntime instance
       */
      WebViewToRuntime.create = function create(properties) {
        return new WebViewToRuntime(properties)
      }

      /**
       * Encodes the specified WebViewToRuntime message. Does not implicitly {@link ipc.webview.WebViewToRuntime.verify|verify} messages.
       * @function encode
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {ipc.webview.IWebViewToRuntime} message WebViewToRuntime message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      WebViewToRuntime.encode = function encode(message, writer) {
        if (!writer) writer = $Writer.create()
        if (
          message.webViewStatus != null &&
          Object.hasOwnProperty.call(message, 'webViewStatus')
        )
          $root.ipc.webview.WebViewStatus.encode(
            message.webViewStatus,
            writer.uint32(/* id 1, wireType 2 =*/ 10).fork()
          ).ldelim()
        return writer
      }

      /**
       * Encodes the specified WebViewToRuntime message, length delimited. Does not implicitly {@link ipc.webview.WebViewToRuntime.verify|verify} messages.
       * @function encodeDelimited
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {ipc.webview.IWebViewToRuntime} message WebViewToRuntime message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      WebViewToRuntime.encodeDelimited = function encodeDelimited(
        message,
        writer
      ) {
        return this.encode(message, writer).ldelim()
      }

      /**
       * Decodes a WebViewToRuntime message from the specified reader or buffer.
       * @function decode
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @param {number} [length] Message length if known beforehand
       * @returns {ipc.webview.WebViewToRuntime} WebViewToRuntime
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      WebViewToRuntime.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader)) reader = $Reader.create(reader)
        let end = length === undefined ? reader.len : reader.pos + length,
          message = new $root.ipc.webview.WebViewToRuntime()
        while (reader.pos < end) {
          let tag = reader.uint32()
          switch (tag >>> 3) {
            case 1:
              message.webViewStatus = $root.ipc.webview.WebViewStatus.decode(
                reader,
                reader.uint32()
              )
              break
            default:
              reader.skipType(tag & 7)
              break
          }
        }
        return message
      }

      /**
       * Decodes a WebViewToRuntime message from the specified reader or buffer, length delimited.
       * @function decodeDelimited
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @returns {ipc.webview.WebViewToRuntime} WebViewToRuntime
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      WebViewToRuntime.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader)) reader = new $Reader(reader)
        return this.decode(reader, reader.uint32())
      }

      /**
       * Verifies a WebViewToRuntime message.
       * @function verify
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {Object.<string,*>} message Plain object to verify
       * @returns {string|null} `null` if valid, otherwise the reason why it is not
       */
      WebViewToRuntime.verify = function verify(message) {
        if (typeof message !== 'object' || message === null)
          return 'object expected'
        let properties = {}
        if (
          message.webViewStatus != null &&
          message.hasOwnProperty('webViewStatus')
        ) {
          properties.rpcMsg = 1
          {
            let error = $root.ipc.webview.WebViewStatus.verify(
              message.webViewStatus
            )
            if (error) return 'webViewStatus.' + error
          }
        }
        return null
      }

      /**
       * Creates a WebViewToRuntime message from a plain object. Also converts values to their respective internal types.
       * @function fromObject
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {Object.<string,*>} object Plain object
       * @returns {ipc.webview.WebViewToRuntime} WebViewToRuntime
       */
      WebViewToRuntime.fromObject = function fromObject(object) {
        if (object instanceof $root.ipc.webview.WebViewToRuntime) return object
        let message = new $root.ipc.webview.WebViewToRuntime()
        if (object.webViewStatus != null) {
          if (typeof object.webViewStatus !== 'object')
            throw TypeError(
              '.ipc.webview.WebViewToRuntime.webViewStatus: object expected'
            )
          message.webViewStatus = $root.ipc.webview.WebViewStatus.fromObject(
            object.webViewStatus
          )
        }
        return message
      }

      /**
       * Creates a plain object from a WebViewToRuntime message. Also converts values to other types if specified.
       * @function toObject
       * @memberof ipc.webview.WebViewToRuntime
       * @static
       * @param {ipc.webview.WebViewToRuntime} message WebViewToRuntime
       * @param {$protobuf.IConversionOptions} [options] Conversion options
       * @returns {Object.<string,*>} Plain object
       */
      WebViewToRuntime.toObject = function toObject(message, options) {
        if (!options) options = {}
        let object = {}
        if (
          message.webViewStatus != null &&
          message.hasOwnProperty('webViewStatus')
        ) {
          object.webViewStatus = $root.ipc.webview.WebViewStatus.toObject(
            message.webViewStatus,
            options
          )
          if (options.oneofs) object.rpcMsg = 'webViewStatus'
        }
        return object
      }

      /**
       * Converts this WebViewToRuntime to JSON.
       * @function toJSON
       * @memberof ipc.webview.WebViewToRuntime
       * @instance
       * @returns {Object.<string,*>} JSON object
       */
      WebViewToRuntime.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions)
      }

      return WebViewToRuntime
    })()

    webview.CreateWebView = (function () {
      /**
       * Properties of a CreateWebView.
       * @memberof ipc.webview
       * @interface ICreateWebView
       * @property {string|null} [id] CreateWebView id
       */

      /**
       * Constructs a new CreateWebView.
       * @memberof ipc.webview
       * @classdesc Represents a CreateWebView.
       * @implements ICreateWebView
       * @constructor
       * @param {ipc.webview.ICreateWebView=} [properties] Properties to set
       */
      function CreateWebView(properties) {
        if (properties)
          for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
            if (properties[keys[i]] != null) this[keys[i]] = properties[keys[i]]
      }

      /**
       * CreateWebView id.
       * @member {string} id
       * @memberof ipc.webview.CreateWebView
       * @instance
       */
      CreateWebView.prototype.id = ''

      /**
       * Creates a new CreateWebView instance using the specified properties.
       * @function create
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {ipc.webview.ICreateWebView=} [properties] Properties to set
       * @returns {ipc.webview.CreateWebView} CreateWebView instance
       */
      CreateWebView.create = function create(properties) {
        return new CreateWebView(properties)
      }

      /**
       * Encodes the specified CreateWebView message. Does not implicitly {@link ipc.webview.CreateWebView.verify|verify} messages.
       * @function encode
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {ipc.webview.ICreateWebView} message CreateWebView message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      CreateWebView.encode = function encode(message, writer) {
        if (!writer) writer = $Writer.create()
        if (message.id != null && Object.hasOwnProperty.call(message, 'id'))
          writer.uint32(/* id 1, wireType 2 =*/ 10).string(message.id)
        return writer
      }

      /**
       * Encodes the specified CreateWebView message, length delimited. Does not implicitly {@link ipc.webview.CreateWebView.verify|verify} messages.
       * @function encodeDelimited
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {ipc.webview.ICreateWebView} message CreateWebView message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      CreateWebView.encodeDelimited = function encodeDelimited(
        message,
        writer
      ) {
        return this.encode(message, writer).ldelim()
      }

      /**
       * Decodes a CreateWebView message from the specified reader or buffer.
       * @function decode
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @param {number} [length] Message length if known beforehand
       * @returns {ipc.webview.CreateWebView} CreateWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      CreateWebView.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader)) reader = $Reader.create(reader)
        let end = length === undefined ? reader.len : reader.pos + length,
          message = new $root.ipc.webview.CreateWebView()
        while (reader.pos < end) {
          let tag = reader.uint32()
          switch (tag >>> 3) {
            case 1:
              message.id = reader.string()
              break
            default:
              reader.skipType(tag & 7)
              break
          }
        }
        return message
      }

      /**
       * Decodes a CreateWebView message from the specified reader or buffer, length delimited.
       * @function decodeDelimited
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @returns {ipc.webview.CreateWebView} CreateWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      CreateWebView.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader)) reader = new $Reader(reader)
        return this.decode(reader, reader.uint32())
      }

      /**
       * Verifies a CreateWebView message.
       * @function verify
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {Object.<string,*>} message Plain object to verify
       * @returns {string|null} `null` if valid, otherwise the reason why it is not
       */
      CreateWebView.verify = function verify(message) {
        if (typeof message !== 'object' || message === null)
          return 'object expected'
        if (message.id != null && message.hasOwnProperty('id'))
          if (!$util.isString(message.id)) return 'id: string expected'
        return null
      }

      /**
       * Creates a CreateWebView message from a plain object. Also converts values to their respective internal types.
       * @function fromObject
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {Object.<string,*>} object Plain object
       * @returns {ipc.webview.CreateWebView} CreateWebView
       */
      CreateWebView.fromObject = function fromObject(object) {
        if (object instanceof $root.ipc.webview.CreateWebView) return object
        let message = new $root.ipc.webview.CreateWebView()
        if (object.id != null) message.id = String(object.id)
        return message
      }

      /**
       * Creates a plain object from a CreateWebView message. Also converts values to other types if specified.
       * @function toObject
       * @memberof ipc.webview.CreateWebView
       * @static
       * @param {ipc.webview.CreateWebView} message CreateWebView
       * @param {$protobuf.IConversionOptions} [options] Conversion options
       * @returns {Object.<string,*>} Plain object
       */
      CreateWebView.toObject = function toObject(message, options) {
        if (!options) options = {}
        let object = {}
        if (options.defaults) object.id = ''
        if (message.id != null && message.hasOwnProperty('id'))
          object.id = message.id
        return object
      }

      /**
       * Converts this CreateWebView to JSON.
       * @function toJSON
       * @memberof ipc.webview.CreateWebView
       * @instance
       * @returns {Object.<string,*>} JSON object
       */
      CreateWebView.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions)
      }

      return CreateWebView
    })()

    webview.QueryWebViewStatus = (function () {
      /**
       * Properties of a QueryWebViewStatus.
       * @memberof ipc.webview
       * @interface IQueryWebViewStatus
       */

      /**
       * Constructs a new QueryWebViewStatus.
       * @memberof ipc.webview
       * @classdesc Represents a QueryWebViewStatus.
       * @implements IQueryWebViewStatus
       * @constructor
       * @param {ipc.webview.IQueryWebViewStatus=} [properties] Properties to set
       */
      function QueryWebViewStatus(properties) {
        if (properties)
          for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
            if (properties[keys[i]] != null) this[keys[i]] = properties[keys[i]]
      }

      /**
       * Creates a new QueryWebViewStatus instance using the specified properties.
       * @function create
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {ipc.webview.IQueryWebViewStatus=} [properties] Properties to set
       * @returns {ipc.webview.QueryWebViewStatus} QueryWebViewStatus instance
       */
      QueryWebViewStatus.create = function create(properties) {
        return new QueryWebViewStatus(properties)
      }

      /**
       * Encodes the specified QueryWebViewStatus message. Does not implicitly {@link ipc.webview.QueryWebViewStatus.verify|verify} messages.
       * @function encode
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {ipc.webview.IQueryWebViewStatus} message QueryWebViewStatus message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      QueryWebViewStatus.encode = function encode(message, writer) {
        if (!writer) writer = $Writer.create()
        return writer
      }

      /**
       * Encodes the specified QueryWebViewStatus message, length delimited. Does not implicitly {@link ipc.webview.QueryWebViewStatus.verify|verify} messages.
       * @function encodeDelimited
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {ipc.webview.IQueryWebViewStatus} message QueryWebViewStatus message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      QueryWebViewStatus.encodeDelimited = function encodeDelimited(
        message,
        writer
      ) {
        return this.encode(message, writer).ldelim()
      }

      /**
       * Decodes a QueryWebViewStatus message from the specified reader or buffer.
       * @function decode
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @param {number} [length] Message length if known beforehand
       * @returns {ipc.webview.QueryWebViewStatus} QueryWebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      QueryWebViewStatus.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader)) reader = $Reader.create(reader)
        let end = length === undefined ? reader.len : reader.pos + length,
          message = new $root.ipc.webview.QueryWebViewStatus()
        while (reader.pos < end) {
          let tag = reader.uint32()
          switch (tag >>> 3) {
            default:
              reader.skipType(tag & 7)
              break
          }
        }
        return message
      }

      /**
       * Decodes a QueryWebViewStatus message from the specified reader or buffer, length delimited.
       * @function decodeDelimited
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @returns {ipc.webview.QueryWebViewStatus} QueryWebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      QueryWebViewStatus.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader)) reader = new $Reader(reader)
        return this.decode(reader, reader.uint32())
      }

      /**
       * Verifies a QueryWebViewStatus message.
       * @function verify
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {Object.<string,*>} message Plain object to verify
       * @returns {string|null} `null` if valid, otherwise the reason why it is not
       */
      QueryWebViewStatus.verify = function verify(message) {
        if (typeof message !== 'object' || message === null)
          return 'object expected'
        return null
      }

      /**
       * Creates a QueryWebViewStatus message from a plain object. Also converts values to their respective internal types.
       * @function fromObject
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {Object.<string,*>} object Plain object
       * @returns {ipc.webview.QueryWebViewStatus} QueryWebViewStatus
       */
      QueryWebViewStatus.fromObject = function fromObject(object) {
        if (object instanceof $root.ipc.webview.QueryWebViewStatus)
          return object
        return new $root.ipc.webview.QueryWebViewStatus()
      }

      /**
       * Creates a plain object from a QueryWebViewStatus message. Also converts values to other types if specified.
       * @function toObject
       * @memberof ipc.webview.QueryWebViewStatus
       * @static
       * @param {ipc.webview.QueryWebViewStatus} message QueryWebViewStatus
       * @param {$protobuf.IConversionOptions} [options] Conversion options
       * @returns {Object.<string,*>} Plain object
       */
      QueryWebViewStatus.toObject = function toObject() {
        return {}
      }

      /**
       * Converts this QueryWebViewStatus to JSON.
       * @function toJSON
       * @memberof ipc.webview.QueryWebViewStatus
       * @instance
       * @returns {Object.<string,*>} JSON object
       */
      QueryWebViewStatus.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions)
      }

      return QueryWebViewStatus
    })()

    webview.WebViewStatus = (function () {
      /**
       * Properties of a WebViewStatus.
       * @memberof ipc.webview
       * @interface IWebViewStatus
       * @property {string|null} [id] WebViewStatus id
       * @property {boolean|null} [isRoot] WebViewStatus isRoot
       */

      /**
       * Constructs a new WebViewStatus.
       * @memberof ipc.webview
       * @classdesc Represents a WebViewStatus.
       * @implements IWebViewStatus
       * @constructor
       * @param {ipc.webview.IWebViewStatus=} [properties] Properties to set
       */
      function WebViewStatus(properties) {
        if (properties)
          for (let keys = Object.keys(properties), i = 0; i < keys.length; ++i)
            if (properties[keys[i]] != null) this[keys[i]] = properties[keys[i]]
      }

      /**
       * WebViewStatus id.
       * @member {string} id
       * @memberof ipc.webview.WebViewStatus
       * @instance
       */
      WebViewStatus.prototype.id = ''

      /**
       * WebViewStatus isRoot.
       * @member {boolean} isRoot
       * @memberof ipc.webview.WebViewStatus
       * @instance
       */
      WebViewStatus.prototype.isRoot = false

      /**
       * Creates a new WebViewStatus instance using the specified properties.
       * @function create
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {ipc.webview.IWebViewStatus=} [properties] Properties to set
       * @returns {ipc.webview.WebViewStatus} WebViewStatus instance
       */
      WebViewStatus.create = function create(properties) {
        return new WebViewStatus(properties)
      }

      /**
       * Encodes the specified WebViewStatus message. Does not implicitly {@link ipc.webview.WebViewStatus.verify|verify} messages.
       * @function encode
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {ipc.webview.IWebViewStatus} message WebViewStatus message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      WebViewStatus.encode = function encode(message, writer) {
        if (!writer) writer = $Writer.create()
        if (message.id != null && Object.hasOwnProperty.call(message, 'id'))
          writer.uint32(/* id 1, wireType 2 =*/ 10).string(message.id)
        if (
          message.isRoot != null &&
          Object.hasOwnProperty.call(message, 'isRoot')
        )
          writer.uint32(/* id 2, wireType 0 =*/ 16).bool(message.isRoot)
        return writer
      }

      /**
       * Encodes the specified WebViewStatus message, length delimited. Does not implicitly {@link ipc.webview.WebViewStatus.verify|verify} messages.
       * @function encodeDelimited
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {ipc.webview.IWebViewStatus} message WebViewStatus message or plain object to encode
       * @param {$protobuf.Writer} [writer] Writer to encode to
       * @returns {$protobuf.Writer} Writer
       */
      WebViewStatus.encodeDelimited = function encodeDelimited(
        message,
        writer
      ) {
        return this.encode(message, writer).ldelim()
      }

      /**
       * Decodes a WebViewStatus message from the specified reader or buffer.
       * @function decode
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @param {number} [length] Message length if known beforehand
       * @returns {ipc.webview.WebViewStatus} WebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      WebViewStatus.decode = function decode(reader, length) {
        if (!(reader instanceof $Reader)) reader = $Reader.create(reader)
        let end = length === undefined ? reader.len : reader.pos + length,
          message = new $root.ipc.webview.WebViewStatus()
        while (reader.pos < end) {
          let tag = reader.uint32()
          switch (tag >>> 3) {
            case 1:
              message.id = reader.string()
              break
            case 2:
              message.isRoot = reader.bool()
              break
            default:
              reader.skipType(tag & 7)
              break
          }
        }
        return message
      }

      /**
       * Decodes a WebViewStatus message from the specified reader or buffer, length delimited.
       * @function decodeDelimited
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {$protobuf.Reader|Uint8Array} reader Reader or buffer to decode from
       * @returns {ipc.webview.WebViewStatus} WebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      WebViewStatus.decodeDelimited = function decodeDelimited(reader) {
        if (!(reader instanceof $Reader)) reader = new $Reader(reader)
        return this.decode(reader, reader.uint32())
      }

      /**
       * Verifies a WebViewStatus message.
       * @function verify
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {Object.<string,*>} message Plain object to verify
       * @returns {string|null} `null` if valid, otherwise the reason why it is not
       */
      WebViewStatus.verify = function verify(message) {
        if (typeof message !== 'object' || message === null)
          return 'object expected'
        if (message.id != null && message.hasOwnProperty('id'))
          if (!$util.isString(message.id)) return 'id: string expected'
        if (message.isRoot != null && message.hasOwnProperty('isRoot'))
          if (typeof message.isRoot !== 'boolean')
            return 'isRoot: boolean expected'
        return null
      }

      /**
       * Creates a WebViewStatus message from a plain object. Also converts values to their respective internal types.
       * @function fromObject
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {Object.<string,*>} object Plain object
       * @returns {ipc.webview.WebViewStatus} WebViewStatus
       */
      WebViewStatus.fromObject = function fromObject(object) {
        if (object instanceof $root.ipc.webview.WebViewStatus) return object
        let message = new $root.ipc.webview.WebViewStatus()
        if (object.id != null) message.id = String(object.id)
        if (object.isRoot != null) message.isRoot = Boolean(object.isRoot)
        return message
      }

      /**
       * Creates a plain object from a WebViewStatus message. Also converts values to other types if specified.
       * @function toObject
       * @memberof ipc.webview.WebViewStatus
       * @static
       * @param {ipc.webview.WebViewStatus} message WebViewStatus
       * @param {$protobuf.IConversionOptions} [options] Conversion options
       * @returns {Object.<string,*>} Plain object
       */
      WebViewStatus.toObject = function toObject(message, options) {
        if (!options) options = {}
        let object = {}
        if (options.defaults) {
          object.id = ''
          object.isRoot = false
        }
        if (message.id != null && message.hasOwnProperty('id'))
          object.id = message.id
        if (message.isRoot != null && message.hasOwnProperty('isRoot'))
          object.isRoot = message.isRoot
        return object
      }

      /**
       * Converts this WebViewStatus to JSON.
       * @function toJSON
       * @memberof ipc.webview.WebViewStatus
       * @instance
       * @returns {Object.<string,*>} JSON object
       */
      WebViewStatus.prototype.toJSON = function toJSON() {
        return this.constructor.toObject(this, $protobuf.util.toJSONOptions)
      }

      return WebViewStatus
    })()

    return webview
  })()

  return ipc
})())

export { $root as default }
