import { TextareaControl } from '@wordpress/components';
import type { RefObject, KeyboardEvent } from 'react';

interface Props {
  value: string;
  onChange: (next: string) => void;
  onSubmit: () => void;
  disabled: boolean;
  textareaRef: RefObject<HTMLTextAreaElement>;
  placeholder?: string;
}

// MessageInput is the textarea + submit-on-Enter at the bottom of the
// drawer. WPDS TextareaControl with __nextHasNoMarginBottom + a
// keyboard handler that submits on Enter (Shift+Enter inserts a
// newline, matching every other AI chat surface).
export default function MessageInput({
  value,
  onChange,
  onSubmit,
  disabled,
  textareaRef,
  placeholder = 'Ask Chief of Staff…',
}: Props) {
  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (!disabled) onSubmit();
    }
  };

  return (
    <div className="wa-chat-input">
      <TextareaControl
        __nextHasNoMarginBottom
        label="Ask"
        hideLabelFromVision
        rows={2}
        value={value}
        placeholder={placeholder}
        onChange={onChange}
        ref={textareaRef as never}
        onKeyDown={handleKeyDown as never}
        disabled={disabled}
      />
      <div className="wa-chat-input__hint">
        Enter to send · Shift+Enter for newline · Esc to close
      </div>
    </div>
  );
}
