// Upstream: https://github.com/wojtekmaj/merge-refs/blob/main/src/index.ts
// This file is subject to the MIT license of wojtekmaj/merge-refs.

/**
 * Merges multiple React refs into a single ref that updates all input refs.
 * This is useful when you need to pass multiple refs to a single element,
 * such as combining a forwarded ref with a local ref.
 *
 * Supports three types of refs:
 * - Function refs (ref callback)
 * - Object refs (from createRef)
 * - Object refs (from useRef)
 *
 * @example
 * ```tsx
 * function MyComponent({ forwardedRef }) {
 *   const localRef = useRef(null);
 *   return <div ref={useMergeRefs(forwardedRef, localRef)} />;
 * }
 * ```
 *
 * @param {...(React.Ref<T> | undefined)} inputRefs - Any number of React refs to merge.
 *        Undefined refs are filtered out.
 * @returns {React.Ref<T> | React.RefCallback<T>} A single ref that updates all input refs.
 *          Returns null if no valid refs are provided, or the single ref if only one is provided.
 */
export function useMergeRefs<T>(
  ...inputRefs: (React.Ref<T> | undefined)[]
): React.Ref<T> | React.RefCallback<T> {
  const filteredInputRefs = inputRefs.filter(Boolean)

  if (filteredInputRefs.length <= 1) {
    const firstRef = filteredInputRefs[0]

    return firstRef || null
  }

  return function mergedRefs(ref) {
    for (const inputRef of filteredInputRefs) {
      if (typeof inputRef === 'function') {
        inputRef(ref)
      } else if (inputRef) {
        ;(inputRef as React.RefObject<T | null>).current = ref
      }
    }
  }
}
