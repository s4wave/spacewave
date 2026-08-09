import { useCallback, useState, type ReactNode } from 'react'

import { PlanFaqItem } from './PlanFaqItem.js'

// FaqAccordion renders a list of FAQ items as an accordion.
export function FaqAccordion({
  items,
}: {
  items: { question: string; answer: ReactNode }[]
}) {
  const [openIndex, setOpenIndex] = useState<number>(-1)
  const handleToggle = useCallback((index: number) => {
    setOpenIndex((previous) => (previous === index ? -1 : index))
  }, [])

  return (
    <div className="flex flex-col gap-2">
      {items.map((item, index) => (
        <PlanFaqItem
          key={item.question}
          question={item.question}
          answer={item.answer}
          isOpen={index === openIndex}
          onToggle={() => handleToggle(index)}
        />
      ))}
    </div>
  )
}
