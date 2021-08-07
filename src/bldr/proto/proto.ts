import * as $protobuf from 'protobufjs'
/** Namespace ipc. */
export namespace ipc {
  /** Namespace webview. */
  namespace webview {
    /** Properties of a RuntimeToWebView. */
    interface IRuntimeToWebView {
      /** RuntimeToWebView createWebView */
      createWebView?: ipc.webview.ICreateWebView | null

      /** RuntimeToWebView queryWebViewStatus */
      queryWebViewStatus?: ipc.webview.IQueryWebViewStatus | null
    }

    /** Represents a RuntimeToWebView. */
    class RuntimeToWebView implements IRuntimeToWebView {
      /**
       * Constructs a new RuntimeToWebView.
       * @param [properties] Properties to set
       */
      constructor(properties?: ipc.webview.IRuntimeToWebView)

      /** RuntimeToWebView createWebView. */
      public createWebView?: ipc.webview.ICreateWebView | null

      /** RuntimeToWebView queryWebViewStatus. */
      public queryWebViewStatus?: ipc.webview.IQueryWebViewStatus | null

      /** RuntimeToWebView rpcMsg. */
      public rpcMsg?: 'createWebView' | 'queryWebViewStatus'

      /**
       * Creates a new RuntimeToWebView instance using the specified properties.
       * @param [properties] Properties to set
       * @returns RuntimeToWebView instance
       */
      public static create(
        properties?: ipc.webview.IRuntimeToWebView
      ): ipc.webview.RuntimeToWebView

      /**
       * Encodes the specified RuntimeToWebView message. Does not implicitly {@link ipc.webview.RuntimeToWebView.verify|verify} messages.
       * @param message RuntimeToWebView message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encode(
        message: ipc.webview.IRuntimeToWebView,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Encodes the specified RuntimeToWebView message, length delimited. Does not implicitly {@link ipc.webview.RuntimeToWebView.verify|verify} messages.
       * @param message RuntimeToWebView message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encodeDelimited(
        message: ipc.webview.IRuntimeToWebView,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Decodes a RuntimeToWebView message from the specified reader or buffer.
       * @param reader Reader or buffer to decode from
       * @param [length] Message length if known beforehand
       * @returns RuntimeToWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decode(
        reader: $protobuf.Reader | Uint8Array,
        length?: number
      ): ipc.webview.RuntimeToWebView

      /**
       * Decodes a RuntimeToWebView message from the specified reader or buffer, length delimited.
       * @param reader Reader or buffer to decode from
       * @returns RuntimeToWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decodeDelimited(
        reader: $protobuf.Reader | Uint8Array
      ): ipc.webview.RuntimeToWebView

      /**
       * Verifies a RuntimeToWebView message.
       * @param message Plain object to verify
       * @returns `null` if valid, otherwise the reason why it is not
       */
      public static verify(message: { [k: string]: any }): string | null

      /**
       * Creates a RuntimeToWebView message from a plain object. Also converts values to their respective internal types.
       * @param object Plain object
       * @returns RuntimeToWebView
       */
      public static fromObject(object: {
        [k: string]: any
      }): ipc.webview.RuntimeToWebView

      /**
       * Creates a plain object from a RuntimeToWebView message. Also converts values to other types if specified.
       * @param message RuntimeToWebView
       * @param [options] Conversion options
       * @returns Plain object
       */
      public static toObject(
        message: ipc.webview.RuntimeToWebView,
        options?: $protobuf.IConversionOptions
      ): { [k: string]: any }

      /**
       * Converts this RuntimeToWebView to JSON.
       * @returns JSON object
       */
      public toJSON(): { [k: string]: any }
    }

    /** Properties of a WebViewToRuntime. */
    interface IWebViewToRuntime {
      /** WebViewToRuntime webViewStatus */
      webViewStatus?: ipc.webview.IWebViewStatus | null
    }

    /** Represents a WebViewToRuntime. */
    class WebViewToRuntime implements IWebViewToRuntime {
      /**
       * Constructs a new WebViewToRuntime.
       * @param [properties] Properties to set
       */
      constructor(properties?: ipc.webview.IWebViewToRuntime)

      /** WebViewToRuntime webViewStatus. */
      public webViewStatus?: ipc.webview.IWebViewStatus | null

      /** WebViewToRuntime rpcMsg. */
      public rpcMsg?: 'webViewStatus'

      /**
       * Creates a new WebViewToRuntime instance using the specified properties.
       * @param [properties] Properties to set
       * @returns WebViewToRuntime instance
       */
      public static create(
        properties?: ipc.webview.IWebViewToRuntime
      ): ipc.webview.WebViewToRuntime

      /**
       * Encodes the specified WebViewToRuntime message. Does not implicitly {@link ipc.webview.WebViewToRuntime.verify|verify} messages.
       * @param message WebViewToRuntime message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encode(
        message: ipc.webview.IWebViewToRuntime,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Encodes the specified WebViewToRuntime message, length delimited. Does not implicitly {@link ipc.webview.WebViewToRuntime.verify|verify} messages.
       * @param message WebViewToRuntime message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encodeDelimited(
        message: ipc.webview.IWebViewToRuntime,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Decodes a WebViewToRuntime message from the specified reader or buffer.
       * @param reader Reader or buffer to decode from
       * @param [length] Message length if known beforehand
       * @returns WebViewToRuntime
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decode(
        reader: $protobuf.Reader | Uint8Array,
        length?: number
      ): ipc.webview.WebViewToRuntime

      /**
       * Decodes a WebViewToRuntime message from the specified reader or buffer, length delimited.
       * @param reader Reader or buffer to decode from
       * @returns WebViewToRuntime
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decodeDelimited(
        reader: $protobuf.Reader | Uint8Array
      ): ipc.webview.WebViewToRuntime

      /**
       * Verifies a WebViewToRuntime message.
       * @param message Plain object to verify
       * @returns `null` if valid, otherwise the reason why it is not
       */
      public static verify(message: { [k: string]: any }): string | null

      /**
       * Creates a WebViewToRuntime message from a plain object. Also converts values to their respective internal types.
       * @param object Plain object
       * @returns WebViewToRuntime
       */
      public static fromObject(object: {
        [k: string]: any
      }): ipc.webview.WebViewToRuntime

      /**
       * Creates a plain object from a WebViewToRuntime message. Also converts values to other types if specified.
       * @param message WebViewToRuntime
       * @param [options] Conversion options
       * @returns Plain object
       */
      public static toObject(
        message: ipc.webview.WebViewToRuntime,
        options?: $protobuf.IConversionOptions
      ): { [k: string]: any }

      /**
       * Converts this WebViewToRuntime to JSON.
       * @returns JSON object
       */
      public toJSON(): { [k: string]: any }
    }

    /** Properties of a CreateWebView. */
    interface ICreateWebView {
      /** CreateWebView id */
      id?: string | null
    }

    /** Represents a CreateWebView. */
    class CreateWebView implements ICreateWebView {
      /**
       * Constructs a new CreateWebView.
       * @param [properties] Properties to set
       */
      constructor(properties?: ipc.webview.ICreateWebView)

      /** CreateWebView id. */
      public id: string

      /**
       * Creates a new CreateWebView instance using the specified properties.
       * @param [properties] Properties to set
       * @returns CreateWebView instance
       */
      public static create(
        properties?: ipc.webview.ICreateWebView
      ): ipc.webview.CreateWebView

      /**
       * Encodes the specified CreateWebView message. Does not implicitly {@link ipc.webview.CreateWebView.verify|verify} messages.
       * @param message CreateWebView message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encode(
        message: ipc.webview.ICreateWebView,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Encodes the specified CreateWebView message, length delimited. Does not implicitly {@link ipc.webview.CreateWebView.verify|verify} messages.
       * @param message CreateWebView message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encodeDelimited(
        message: ipc.webview.ICreateWebView,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Decodes a CreateWebView message from the specified reader or buffer.
       * @param reader Reader or buffer to decode from
       * @param [length] Message length if known beforehand
       * @returns CreateWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decode(
        reader: $protobuf.Reader | Uint8Array,
        length?: number
      ): ipc.webview.CreateWebView

      /**
       * Decodes a CreateWebView message from the specified reader or buffer, length delimited.
       * @param reader Reader or buffer to decode from
       * @returns CreateWebView
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decodeDelimited(
        reader: $protobuf.Reader | Uint8Array
      ): ipc.webview.CreateWebView

      /**
       * Verifies a CreateWebView message.
       * @param message Plain object to verify
       * @returns `null` if valid, otherwise the reason why it is not
       */
      public static verify(message: { [k: string]: any }): string | null

      /**
       * Creates a CreateWebView message from a plain object. Also converts values to their respective internal types.
       * @param object Plain object
       * @returns CreateWebView
       */
      public static fromObject(object: {
        [k: string]: any
      }): ipc.webview.CreateWebView

      /**
       * Creates a plain object from a CreateWebView message. Also converts values to other types if specified.
       * @param message CreateWebView
       * @param [options] Conversion options
       * @returns Plain object
       */
      public static toObject(
        message: ipc.webview.CreateWebView,
        options?: $protobuf.IConversionOptions
      ): { [k: string]: any }

      /**
       * Converts this CreateWebView to JSON.
       * @returns JSON object
       */
      public toJSON(): { [k: string]: any }
    }

    /** Properties of a QueryWebViewStatus. */
    interface IQueryWebViewStatus {}

    /** Represents a QueryWebViewStatus. */
    class QueryWebViewStatus implements IQueryWebViewStatus {
      /**
       * Constructs a new QueryWebViewStatus.
       * @param [properties] Properties to set
       */
      constructor(properties?: ipc.webview.IQueryWebViewStatus)

      /**
       * Creates a new QueryWebViewStatus instance using the specified properties.
       * @param [properties] Properties to set
       * @returns QueryWebViewStatus instance
       */
      public static create(
        properties?: ipc.webview.IQueryWebViewStatus
      ): ipc.webview.QueryWebViewStatus

      /**
       * Encodes the specified QueryWebViewStatus message. Does not implicitly {@link ipc.webview.QueryWebViewStatus.verify|verify} messages.
       * @param message QueryWebViewStatus message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encode(
        message: ipc.webview.IQueryWebViewStatus,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Encodes the specified QueryWebViewStatus message, length delimited. Does not implicitly {@link ipc.webview.QueryWebViewStatus.verify|verify} messages.
       * @param message QueryWebViewStatus message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encodeDelimited(
        message: ipc.webview.IQueryWebViewStatus,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Decodes a QueryWebViewStatus message from the specified reader or buffer.
       * @param reader Reader or buffer to decode from
       * @param [length] Message length if known beforehand
       * @returns QueryWebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decode(
        reader: $protobuf.Reader | Uint8Array,
        length?: number
      ): ipc.webview.QueryWebViewStatus

      /**
       * Decodes a QueryWebViewStatus message from the specified reader or buffer, length delimited.
       * @param reader Reader or buffer to decode from
       * @returns QueryWebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decodeDelimited(
        reader: $protobuf.Reader | Uint8Array
      ): ipc.webview.QueryWebViewStatus

      /**
       * Verifies a QueryWebViewStatus message.
       * @param message Plain object to verify
       * @returns `null` if valid, otherwise the reason why it is not
       */
      public static verify(message: { [k: string]: any }): string | null

      /**
       * Creates a QueryWebViewStatus message from a plain object. Also converts values to their respective internal types.
       * @param object Plain object
       * @returns QueryWebViewStatus
       */
      public static fromObject(object: {
        [k: string]: any
      }): ipc.webview.QueryWebViewStatus

      /**
       * Creates a plain object from a QueryWebViewStatus message. Also converts values to other types if specified.
       * @param message QueryWebViewStatus
       * @param [options] Conversion options
       * @returns Plain object
       */
      public static toObject(
        message: ipc.webview.QueryWebViewStatus,
        options?: $protobuf.IConversionOptions
      ): { [k: string]: any }

      /**
       * Converts this QueryWebViewStatus to JSON.
       * @returns JSON object
       */
      public toJSON(): { [k: string]: any }
    }

    /** Properties of a WebViewStatus. */
    interface IWebViewStatus {
      /** WebViewStatus id */
      id?: string | null

      /** WebViewStatus isRoot */
      isRoot?: boolean | null
    }

    /** Represents a WebViewStatus. */
    class WebViewStatus implements IWebViewStatus {
      /**
       * Constructs a new WebViewStatus.
       * @param [properties] Properties to set
       */
      constructor(properties?: ipc.webview.IWebViewStatus)

      /** WebViewStatus id. */
      public id: string

      /** WebViewStatus isRoot. */
      public isRoot: boolean

      /**
       * Creates a new WebViewStatus instance using the specified properties.
       * @param [properties] Properties to set
       * @returns WebViewStatus instance
       */
      public static create(
        properties?: ipc.webview.IWebViewStatus
      ): ipc.webview.WebViewStatus

      /**
       * Encodes the specified WebViewStatus message. Does not implicitly {@link ipc.webview.WebViewStatus.verify|verify} messages.
       * @param message WebViewStatus message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encode(
        message: ipc.webview.IWebViewStatus,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Encodes the specified WebViewStatus message, length delimited. Does not implicitly {@link ipc.webview.WebViewStatus.verify|verify} messages.
       * @param message WebViewStatus message or plain object to encode
       * @param [writer] Writer to encode to
       * @returns Writer
       */
      public static encodeDelimited(
        message: ipc.webview.IWebViewStatus,
        writer?: $protobuf.Writer
      ): $protobuf.Writer

      /**
       * Decodes a WebViewStatus message from the specified reader or buffer.
       * @param reader Reader or buffer to decode from
       * @param [length] Message length if known beforehand
       * @returns WebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decode(
        reader: $protobuf.Reader | Uint8Array,
        length?: number
      ): ipc.webview.WebViewStatus

      /**
       * Decodes a WebViewStatus message from the specified reader or buffer, length delimited.
       * @param reader Reader or buffer to decode from
       * @returns WebViewStatus
       * @throws {Error} If the payload is not a reader or valid buffer
       * @throws {$protobuf.util.ProtocolError} If required fields are missing
       */
      public static decodeDelimited(
        reader: $protobuf.Reader | Uint8Array
      ): ipc.webview.WebViewStatus

      /**
       * Verifies a WebViewStatus message.
       * @param message Plain object to verify
       * @returns `null` if valid, otherwise the reason why it is not
       */
      public static verify(message: { [k: string]: any }): string | null

      /**
       * Creates a WebViewStatus message from a plain object. Also converts values to their respective internal types.
       * @param object Plain object
       * @returns WebViewStatus
       */
      public static fromObject(object: {
        [k: string]: any
      }): ipc.webview.WebViewStatus

      /**
       * Creates a plain object from a WebViewStatus message. Also converts values to other types if specified.
       * @param message WebViewStatus
       * @param [options] Conversion options
       * @returns Plain object
       */
      public static toObject(
        message: ipc.webview.WebViewStatus,
        options?: $protobuf.IConversionOptions
      ): { [k: string]: any }

      /**
       * Converts this WebViewStatus to JSON.
       * @returns JSON object
       */
      public toJSON(): { [k: string]: any }
    }
  }
}
