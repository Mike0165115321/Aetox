
# Accessibility Compliance

Master accessibility implementation to create inclusive experiences that work for everyone, including users with disabilities.

## When to Use This Skill

- Implementing WCAG 2.2 Level AA or AAA compliance
- Building screen reader accessible interfaces
- Adding keyboard navigation to interactive components
- Implementing focus management and focus trapping
- Creating accessible forms with proper labeling
- Supporting reduced motion and high contrast preferences
- Building mobile accessibility features (iOS VoiceOver, Android TalkBack)
- Conducting accessibility audits and fixing violations

## Detailed patterns and worked examples

The detailed patterns follow below in this file.

## Best Practices

1. **Use Semantic HTML**: Prefer native elements over ARIA when possible
2. **Test with Real Users**: Include people with disabilities in user testing
3. **Keyboard First**: Design interactions to work without a mouse
4. **Don't Disable Focus Styles**: Style them, don't remove them
5. **Provide Text Alternatives**: All non-text content needs descriptions
6. **Support Zoom**: Content should work at 200% zoom
7. **Announce Changes**: Use live regions for dynamic content
8. **Respect Preferences**: Honor prefers-reduced-motion and prefers-contrast

## Common Issues

- **Missing alt text**: Images without descriptions
- **Poor color contrast**: Text hard to read against background
- **Keyboard traps**: Focus stuck in component
- **Missing labels**: Form inputs without associated labels
- **Auto-playing media**: Content that plays without user initiation
- **Inaccessible custom controls**: Recreating native functionality poorly
- **Missing skip links**: No way to bypass repetitive content
- **Focus order issues**: Tab order doesn't match visual order

## Testing Tools

- **Automated**: axe DevTools, WAVE, Lighthouse
- **Manual**: VoiceOver (macOS/iOS), NVDA/JAWS (Windows), TalkBack (Android)
- **Simulators**: NoCoffee (vision), Silktide (various disabilities)


---

## อ้างอิง: aria-patterns

# ARIA Patterns and Best Practices

## Overview

ARIA (Accessible Rich Internet Applications) provides attributes to enhance accessibility when native HTML semantics are insufficient. The first rule of ARIA is: don't use ARIA if native HTML can do the job.

## ARIA Fundamentals

### Roles

Roles define what an element is or does.

```tsx
// Widget roles
<div role="button">Click me</div>
<div role="checkbox" aria-checked="true">Option</div>
<div role="slider" aria-valuenow="50">Volume</div>

// Landmark roles (prefer semantic HTML)
<div role="main">...</div>      // Better: <main>
<div role="navigation">...</div> // Better: <nav>
<div role="banner">...</div>     // Better: <header>

// Document structure roles
<div role="region" aria-label="Featured">...</div>
<div role="group" aria-label="Formatting options">...</div>
```

### States and Properties

States indicate current conditions; properties describe relationships.

```tsx
// States (can change)
aria-checked="true|false|mixed"
aria-disabled="true|false"
aria-expanded="true|false"
aria-hidden="true|false"
aria-pressed="true|false"
aria-selected="true|false"

// Properties (usually static)
aria-label="Accessible name"
aria-labelledby="id-of-label"
aria-describedby="id-of-description"
aria-controls="id-of-controlled-element"
aria-owns="id-of-owned-element"
aria-live="polite|assertive|off"
```

## Common ARIA Patterns

### Accordion

```tsx
function Accordion({ items }) {
  const [openIndex, setOpenIndex] = useState(-1);

  return (
    <div className="accordion">
      {items.map((item, index) => {
        const isOpen = openIndex === index;
        const headingId = `accordion-heading-${index}`;
        const panelId = `accordion-panel-${index}`;

        return (
          <div key={index}>
            <h3>
              <button
                id={headingId}
                aria-expanded={isOpen}
                aria-controls={panelId}
                onClick={() => setOpenIndex(isOpen ? -1 : index)}
              >
                {item.title}
                <span aria-hidden="true">{isOpen ? "−" : "+"}</span>
              </button>
            </h3>
            <div
              id={panelId}
              role="region"
              aria-labelledby={headingId}
              hidden={!isOpen}
            >
              {item.content}
            </div>
          </div>
        );
      })}
    </div>
  );
}
```

### Tabs

```tsx
function Tabs({ tabs }) {
  const [activeIndex, setActiveIndex] = useState(0);
  const tabListRef = useRef(null);

  const handleKeyDown = (e, index) => {
    let newIndex = index;

    switch (e.key) {
      case "ArrowRight":
        newIndex = (index + 1) % tabs.length;
        break;
      case "ArrowLeft":
        newIndex = (index - 1 + tabs.length) % tabs.length;
        break;
      case "Home":
        newIndex = 0;
        break;
      case "End":
        newIndex = tabs.length - 1;
        break;
      default:
        return;
    }

    e.preventDefault();
    setActiveIndex(newIndex);
    tabListRef.current?.children[newIndex]?.focus();
  };

  return (
    <div>
      <div role="tablist" ref={tabListRef} aria-label="Content tabs">
        {tabs.map((tab, index) => (
          <button
            key={index}
            role="tab"
            id={`tab-${index}`}
            aria-selected={index === activeIndex}
            aria-controls={`panel-${index}`}
            tabIndex={index === activeIndex ? 0 : -1}
            onClick={() => setActiveIndex(index)}
            onKeyDown={(e) => handleKeyDown(e, index)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {tabs.map((tab, index) => (
        <div
          key={index}
          role="tabpanel"
          id={`panel-${index}`}
          aria-labelledby={`tab-${index}`}
          hidden={index !== activeIndex}
          tabIndex={0}
        >
          {tab.content}
        </div>
      ))}
    </div>
  );
}
```

### Menu Button

```tsx
function MenuButton({ label, items }) {
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const buttonRef = useRef(null);
  const menuRef = useRef(null);
  const menuId = useId();

  const handleKeyDown = (e) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        if (!isOpen) {
          setIsOpen(true);
          setActiveIndex(0);
        } else {
          setActiveIndex((prev) => Math.min(prev + 1, items.length - 1));
        }
        break;
      case "ArrowUp":
        e.preventDefault();
        setActiveIndex((prev) => Math.max(prev - 1, 0));
        break;
      case "Escape":
        setIsOpen(false);
        buttonRef.current?.focus();
        break;
      case "Enter":
      case " ":
        if (isOpen && activeIndex >= 0) {
          e.preventDefault();
          items[activeIndex].onClick();
          setIsOpen(false);
        }
        break;
    }
  };

  // Focus management
  useEffect(() => {
    if (isOpen && activeIndex >= 0) {
      menuRef.current?.children[activeIndex]?.focus();
    }
  }, [isOpen, activeIndex]);

  return (
    <div>
      <button
        ref={buttonRef}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        aria-controls={menuId}
        onClick={() => setIsOpen(!isOpen)}
        onKeyDown={handleKeyDown}
      >
        {label}
      </button>

      {isOpen && (
        <ul
          ref={menuRef}
          id={menuId}
          role="menu"
          aria-label={label}
          onKeyDown={handleKeyDown}
        >
          {items.map((item, index) => (
            <li
              key={index}
              role="menuitem"
              tabIndex={-1}
              onClick={() => {
                item.onClick();
                setIsOpen(false);
                buttonRef.current?.focus();
              }}
            >
              {item.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
```

### Combobox (Autocomplete)

```tsx
function Combobox({ options, onSelect, placeholder }) {
  const [inputValue, setInputValue] = useState("");
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const inputRef = useRef(null);
  const listboxId = useId();

  const filteredOptions = options.filter((opt) =>
    opt.toLowerCase().includes(inputValue.toLowerCase()),
  );

  const handleKeyDown = (e) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setIsOpen(true);
        setActiveIndex((prev) =>
          Math.min(prev + 1, filteredOptions.length - 1),
        );
        break;
      case "ArrowUp":
        e.preventDefault();
        setActiveIndex((prev) => Math.max(prev - 1, 0));
        break;
      case "Enter":
        if (activeIndex >= 0) {
          e.preventDefault();
          selectOption(filteredOptions[activeIndex]);
        }
        break;
      case "Escape":
        setIsOpen(false);
        setActiveIndex(-1);
        break;
    }
  };

  const selectOption = (option) => {
    setInputValue(option);
    onSelect(option);
    setIsOpen(false);
    setActiveIndex(-1);
  };

  return (
    <div>
      <input
        ref={inputRef}
        type="text"
        role="combobox"
        aria-expanded={isOpen}
        aria-controls={listboxId}
        aria-activedescendant={
          activeIndex >= 0 ? `option-${activeIndex}` : undefined
        }
        aria-autocomplete="list"
        value={inputValue}
        placeholder={placeholder}
        onChange={(e) => {
          setInputValue(e.target.value);
          setIsOpen(true);
          setActiveIndex(-1);
        }}
        onKeyDown={handleKeyDown}
        onFocus={() => setIsOpen(true)}
        onBlur={() => setTimeout(() => setIsOpen(false), 200)}
      />

      {isOpen && filteredOptions.length > 0 && (
        <ul id={listboxId} role="listbox">
          {filteredOptions.map((option, index) => (
            <li
              key={option}
              id={`option-${index}`}
              role="option"
              aria-selected={index === activeIndex}
              onClick={() => selectOption(option)}
              onMouseEnter={() => setActiveIndex(index)}
            >
              {option}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
```

### Alert Dialog

```tsx
function AlertDialog({ isOpen, onConfirm, onCancel, title, message }) {
  const confirmRef = useRef(null);
  const dialogId = useId();
  const titleId = `${dialogId}-title`;
  const descId = `${dialogId}-desc`;

  useEffect(() => {
    if (isOpen) {
      confirmRef.current?.focus();
    }
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <FocusTrap>
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descId}
      >
        <div className="backdrop" onClick={onCancel} />

        <div className="dialog">
          <h2 id={titleId}>{title}</h2>
          <p id={descId}>{message}</p>

          <div className="actions">
            <button onClick={onCancel}>Cancel</button>
            <button ref={confirmRef} onClick={onConfirm}>
              Confirm
            </button>
          </div>
        </div>
      </div>
    </FocusTrap>
  );
}
```

### Toolbar

```tsx
function Toolbar({ items }) {
  const [activeIndex, setActiveIndex] = useState(0);
  const toolbarRef = useRef(null);

  const handleKeyDown = (e) => {
    let newIndex = activeIndex;

    switch (e.key) {
      case "ArrowRight":
        newIndex = (activeIndex + 1) % items.length;
        break;
      case "ArrowLeft":
        newIndex = (activeIndex - 1 + items.length) % items.length;
        break;
      case "Home":
        newIndex = 0;
        break;
      case "End":
        newIndex = items.length - 1;
        break;
      default:
        return;
    }

    e.preventDefault();
    setActiveIndex(newIndex);
    toolbarRef.current?.querySelectorAll("button")[newIndex]?.focus();
  };

  return (
    <div
      ref={toolbarRef}
      role="toolbar"
      aria-label="Text formatting"
      onKeyDown={handleKeyDown}
    >
      {items.map((item, index) => (
        <button
          key={index}
          tabIndex={index === activeIndex ? 0 : -1}
          aria-pressed={item.isActive}
          aria-label={item.label}
          onClick={item.onClick}
        >
          {item.icon}
        </button>
      ))}
    </div>
  );
}
```

## Live Regions

### Polite Announcements

```tsx
// Status messages that don't interrupt
function SearchStatus({ count, query }) {
  return (
    <div role="status" aria-live="polite" aria-atomic="true">
      {count} results found for "{query}"
    </div>
  );
}

// Progress indicator
function LoadingStatus({ progress }) {
  return (
    <div role="status" aria-live="polite">
      Loading: {progress}% complete
    </div>
  );
}
```

### Assertive Announcements

```tsx
// Important errors that should interrupt
function ErrorAlert({ message }) {
  return (
    <div role="alert" aria-live="assertive">
      Error: {message}
    </div>
  );
}

// Form validation summary
function ValidationSummary({ errors }) {
  if (errors.length === 0) return null;

  return (
    <div role="alert" aria-live="assertive">
      <h2>Please fix the following errors:</h2>
      <ul>
        {errors.map((error, index) => (
          <li key={index}>{error}</li>
        ))}
      </ul>
    </div>
  );
}
```

### Log Region

```tsx
// Chat messages or activity log
function ChatLog({ messages }) {
  return (
    <div role="log" aria-live="polite" aria-relevant="additions">
      {messages.map((msg) => (
        <div key={msg.id}>
          <span className="author">{msg.author}:</span>
          <span className="text">{msg.text}</span>
        </div>
      ))}
    </div>
  );
}
```

## Common Mistakes to Avoid

### 1. Redundant ARIA

```tsx
// Bad: role="button" on a button
<button role="button">Click me</button>

// Good: just use button
<button>Click me</button>

// Bad: aria-label duplicating visible text
<button aria-label="Submit form">Submit form</button>

// Good: just use visible text
<button>Submit form</button>
```

### 2. Invalid ARIA

```tsx
// Bad: aria-selected on non-selectable element
<div aria-selected="true">Item</div>

// Good: use with proper role
<div role="option" aria-selected="true">Item</div>

// Bad: aria-expanded without control relationship
<button aria-expanded="true">Menu</button>
<div>Menu content</div>

// Good: with aria-controls
<button aria-expanded="true" aria-controls="menu">Menu</button>
<div id="menu">Menu content</div>
```

### 3. Hidden Content Still Announced

```tsx
// Bad: visually hidden but still in accessibility tree
<div style={{ display: 'none' }}>Hidden content</div>

// Good: properly hidden
<div style={{ display: 'none' }} aria-hidden="true">Hidden content</div>

// Or just use display: none (implicitly hidden)
<div hidden>Hidden content</div>
```

## Resources

- [WAI-ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/)
- [ARIA in HTML](https://www.w3.org/TR/html-aria/)
- [Using ARIA](https://www.w3.org/TR/using-aria/)


---

## อ้างอิง: details

# accessibility-compliance, detailed patterns and worked examples

## Core Capabilities

### 1. WCAG 2.2 Guidelines

- Perceivable: Content must be presentable in different ways
- Operable: Interface must be navigable with keyboard and assistive tech
- Understandable: Content and operation must be clear
- Robust: Content must work with current and future assistive technologies

### 2. ARIA Patterns

- Roles: Define element purpose (button, dialog, navigation)
- States: Indicate current condition (expanded, selected, disabled)
- Properties: Describe relationships and additional info (labelledby, describedby)
- Live regions: Announce dynamic content changes

### 3. Keyboard Navigation

- Focus order and tab sequence
- Focus indicators and visible focus states
- Keyboard shortcuts and hotkeys
- Focus trapping for modals and dialogs

### 4. Screen Reader Support

- Semantic HTML structure
- Alternative text for images
- Proper heading hierarchy
- Skip links and landmarks

### 5. Mobile Accessibility

- Touch target sizing (44x44dp minimum)
- VoiceOver and TalkBack compatibility
- Gesture alternatives
- Dynamic Type support

## Quick Reference

### WCAG 2.2 Success Criteria Checklist

| Level | Criterion | Description                                          |
| ----- | --------- | ---------------------------------------------------- |
| A     | 1.1.1     | Non-text content has text alternatives               |
| A     | 1.3.1     | Info and relationships programmatically determinable |
| A     | 2.1.1     | All functionality keyboard accessible                |
| A     | 2.4.1     | Skip to main content mechanism                       |
| AA    | 1.4.3     | Contrast ratio 4.5:1 (text), 3:1 (large text)        |
| AA    | 1.4.11    | Non-text contrast 3:1                                |
| AA    | 2.4.7     | Focus visible                                        |
| AA    | 2.5.8     | Target size minimum 24x24px (NEW in 2.2)             |
| AAA   | 1.4.6     | Enhanced contrast 7:1                                |
| AAA   | 2.5.5     | Target size minimum 44x44px                          |

## Key Patterns

### Pattern 1: Accessible Button

```tsx
interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary";
  isLoading?: boolean;
}

function AccessibleButton({
  children,
  variant = "primary",
  isLoading = false,
  disabled,
  ...props
}: ButtonProps) {
  return (
    <button
      // Disable when loading
      disabled={disabled || isLoading}
      // Announce loading state to screen readers
      aria-busy={isLoading}
      // Describe the button's current state
      aria-disabled={disabled || isLoading}
      className={cn(
        // Visible focus ring
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2",
        // Minimum touch target size (44x44px)
        "min-h-[44px] min-w-[44px]",
        variant === "primary" && "bg-primary text-primary-foreground",
        (disabled || isLoading) && "opacity-50 cursor-not-allowed",
      )}
      {...props}
    >
      {isLoading ? (
        <>
          <span className="sr-only">Loading</span>
          <Spinner aria-hidden="true" />
        </>
      ) : (
        children
      )}
    </button>
  );
}
```

### Pattern 2: Accessible Modal Dialog

```tsx
import * as React from "react";
import { FocusTrap } from "@headlessui/react";

interface DialogProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
}

function AccessibleDialog({ isOpen, onClose, title, children }: DialogProps) {
  const titleId = React.useId();
  const descriptionId = React.useId();

  // Close on Escape key
  React.useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        onClose();
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  // Prevent body scroll when open
  React.useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = "hidden";
    }
    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={descriptionId}
    >
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/50"
        aria-hidden="true"
        onClick={onClose}
      />

      {/* Focus trap container */}
      <FocusTrap>
        <div className="fixed inset-0 flex items-center justify-center p-4">
          <div className="bg-background rounded-lg shadow-lg max-w-md w-full p-6">
            <h2 id={titleId} className="text-lg font-semibold">
              {title}
            </h2>
            <div id={descriptionId}>{children}</div>
            <button
              onClick={onClose}
              className="absolute top-4 right-4"
              aria-label="Close dialog"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
      </FocusTrap>
    </div>
  );
}
```

### Pattern 3: Accessible Form

```tsx
function AccessibleForm() {
  const [errors, setErrors] = React.useState<Record<string, string>>({});

  return (
    <form aria-describedby="form-errors" noValidate>
      {/* Error summary for screen readers */}
      {Object.keys(errors).length > 0 && (
        <div
          id="form-errors"
          role="alert"
          aria-live="assertive"
          className="bg-destructive/10 border border-destructive p-4 rounded-md mb-4"
        >
          <h2 className="font-semibold text-destructive">
            Please fix the following errors:
          </h2>
          <ul className="list-disc list-inside mt-2">
            {Object.entries(errors).map(([field, message]) => (
              <li key={field}>
                <a href={`#${field}`} className="underline">
                  {message}
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Required field with error */}
      <div className="space-y-2">
        <label htmlFor="email" className="block font-medium">
          Email address
          <span aria-hidden="true" className="text-destructive ml-1">
            *
          </span>
          <span className="sr-only">(required)</span>
        </label>
        <input
          id="email"
          name="email"
          type="email"
          required
          aria-required="true"
          aria-invalid={!!errors.email}
          aria-describedby={errors.email ? "email-error" : "email-hint"}
          className={cn(
            "w-full px-3 py-2 border rounded-md",
            errors.email && "border-destructive",
          )}
        />
        {errors.email ? (
          <p id="email-error" className="text-sm text-destructive" role="alert">
            {errors.email}
          </p>
        ) : (
          <p id="email-hint" className="text-sm text-muted-foreground">
            We'll never share your email.
          </p>
        )}
      </div>

      <button type="submit" className="mt-4">
        Submit
      </button>
    </form>
  );
}
```

### Pattern 4: Skip Navigation Link

```tsx
function SkipLink() {
  return (
    <a
      href="#main-content"
      className={cn(
        // Hidden by default, visible on focus
        "sr-only focus:not-sr-only",
        "focus:absolute focus:top-4 focus:left-4 focus:z-50",
        "focus:bg-background focus:px-4 focus:py-2 focus:rounded-md",
        "focus:ring-2 focus:ring-primary",
      )}
    >
      Skip to main content
    </a>
  );
}

// In layout
function Layout({ children }) {
  return (
    <>
      <SkipLink />
      <header>...</header>
      <nav aria-label="Main navigation">...</nav>
      <main id="main-content" tabIndex={-1}>
        {children}
      </main>
      <footer>...</footer>
    </>
  );
}
```

### Pattern 5: Live Region for Announcements

```tsx
function useAnnounce() {
  const [message, setMessage] = React.useState("");

  const announce = React.useCallback(
    (text: string, priority: "polite" | "assertive" = "polite") => {
      setMessage(""); // Clear first to ensure re-announcement
      setTimeout(() => setMessage(text), 100);
    },
    [],
  );

  const Announcer = () => (
    <div
      role="status"
      aria-live="polite"
      aria-atomic="true"
      className="sr-only"
    >
      {message}
    </div>
  );

  return { announce, Announcer };
}

// Usage
function SearchResults({ results, isLoading }) {
  const { announce, Announcer } = useAnnounce();

  React.useEffect(() => {
    if (!isLoading && results) {
      announce(`${results.length} results found`);
    }
  }, [results, isLoading, announce]);

  return (
    <>
      <Announcer />
      <ul>{/* results */}</ul>
    </>
  );
}
```

## Color Contrast Requirements

```typescript
// Contrast ratio utilities
function getContrastRatio(foreground: string, background: string): number {
  const fgLuminance = getLuminance(foreground);
  const bgLuminance = getLuminance(background);
  const lighter = Math.max(fgLuminance, bgLuminance);
  const darker = Math.min(fgLuminance, bgLuminance);
  return (lighter + 0.05) / (darker + 0.05);
}

// WCAG requirements
const CONTRAST_REQUIREMENTS = {
  // Normal text (<18pt or <14pt bold)
  normalText: {
    AA: 4.5,
    AAA: 7,
  },
  // Large text (>=18pt or >=14pt bold)
  largeText: {
    AA: 3,
    AAA: 4.5,
  },
  // UI components and graphics
  uiComponents: {
    AA: 3,
  },
};
```


---

## อ้างอิง: mobile-accessibility

# Mobile Accessibility

## Overview

Mobile accessibility ensures apps work for users with disabilities on iOS and Android devices. This includes support for screen readers (VoiceOver, TalkBack), motor impairments, and various visual disabilities.

## Touch Target Sizing

### Minimum Sizes

```css
/* WCAG 2.2 Level AA: 24x24px minimum */
.interactive-element {
  min-width: 24px;
  min-height: 24px;
}

/* WCAG 2.2 Level AAA / Apple HIG / Material Design: 44x44dp */
.touch-target {
  min-width: 44px;
  min-height: 44px;
}

/* Android Material Design: 48x48dp recommended */
.android-touch-target {
  min-width: 48px;
  min-height: 48px;
}
```

### Touch Target Spacing

```tsx
// Ensure adequate spacing between touch targets
function ButtonGroup({ buttons }) {
  return (
    <div className="flex gap-3">
      {" "}
      {/* 12px minimum gap */}
      {buttons.map((btn) => (
        <button key={btn.id} className="min-w-[44px] min-h-[44px] px-4 py-2">
          {btn.label}
        </button>
      ))}
    </div>
  );
}

// Expanding hit area without changing visual size
function IconButton({ icon, label, onClick }) {
  return (
    <button
      onClick={onClick}
      aria-label={label}
      className="relative p-3" // Creates 44x44 touch area
    >
      <span className="block w-5 h-5">{icon}</span>
    </button>
  );
}
```

## iOS VoiceOver

### React Native Accessibility Props

```tsx
import { View, Text, TouchableOpacity, AccessibilityInfo } from "react-native";

// Basic accessible button
function AccessibleButton({ onPress, title, hint }) {
  return (
    <TouchableOpacity
      onPress={onPress}
      accessible={true}
      accessibilityLabel={title}
      accessibilityHint={hint}
      accessibilityRole="button"
    >
      <Text>{title}</Text>
    </TouchableOpacity>
  );
}

// Complex component with grouped content
function ProductCard({ product }) {
  return (
    <View
      accessible={true}
      accessibilityLabel={`${product.name}, ${product.price}, ${product.rating} stars`}
      accessibilityRole="button"
      accessibilityActions={[
        { name: "activate", label: "View details" },
        { name: "addToCart", label: "Add to cart" },
      ]}
      onAccessibilityAction={(event) => {
        switch (event.nativeEvent.actionName) {
          case "addToCart":
            addToCart(product);
            break;
          case "activate":
            viewDetails(product);
            break;
        }
      }}
    >
      <Image source={product.image} accessibilityIgnoresInvertColors />
      <Text>{product.name}</Text>
      <Text>{product.price}</Text>
    </View>
  );
}

// Announcing dynamic changes
function Counter() {
  const [count, setCount] = useState(0);

  const increment = () => {
    setCount((prev) => prev + 1);
    AccessibilityInfo.announceForAccessibility(`Count is now ${count + 1}`);
  };

  return (
    <View>
      <Text accessibilityRole="text" accessibilityLiveRegion="polite">
        Count: {count}
      </Text>
      <TouchableOpacity
        onPress={increment}
        accessibilityLabel="Increment"
        accessibilityHint="Increases the counter by one"
      >
        <Text>+</Text>
      </TouchableOpacity>
    </View>
  );
}
```

### SwiftUI Accessibility

```swift
import SwiftUI

struct AccessibleButton: View {
    let title: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Text(title)
        }
        .accessibilityLabel(title)
        .accessibilityHint("Double tap to activate")
        .accessibilityAddTraits(.isButton)
    }
}

struct ProductCard: View {
    let product: Product

    var body: some View {
        VStack {
            AsyncImage(url: product.imageURL)
                .accessibilityHidden(true) // Image is decorative

            Text(product.name)
            Text(product.price.formatted(.currency(code: "USD")))
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(product.name), \(product.price.formatted(.currency(code: "USD")))")
        .accessibilityHint("Double tap to view details")
        .accessibilityAction(named: "Add to cart") {
            addToCart(product)
        }
    }
}

// Custom accessibility rotor
struct DocumentView: View {
    let sections: [Section]

    var body: some View {
        ScrollView {
            ForEach(sections) { section in
                Text(section.title)
                    .font(.headline)
                    .accessibilityAddTraits(.isHeader)
                Text(section.content)
            }
        }
        .accessibilityRotor("Headings") {
            ForEach(sections) { section in
                AccessibilityRotorEntry(section.title, id: section.id)
            }
        }
    }
}
```

## Android TalkBack

### Jetpack Compose Accessibility

```kotlin
import androidx.compose.ui.semantics.*

@Composable
fun AccessibleButton(
    onClick: () -> Unit,
    text: String,
    enabled: Boolean = true
) {
    Button(
        onClick = onClick,
        enabled = enabled,
        modifier = Modifier.semantics {
            contentDescription = text
            role = Role.Button
            if (!enabled) {
                disabled()
            }
        }
    ) {
        Text(text)
    }
}

@Composable
fun ProductCard(product: Product) {
    Card(
        modifier = Modifier
            .semantics(mergeDescendants = true) {
                contentDescription = "${product.name}, ${product.formattedPrice}"
                customActions = listOf(
                    CustomAccessibilityAction("Add to cart") {
                        addToCart(product)
                        true
                    }
                )
            }
            .clickable { navigateToDetails(product) }
    ) {
        Image(
            painter = painterResource(product.imageRes),
            contentDescription = null, // Decorative
            modifier = Modifier.semantics { invisibleToUser() }
        )
        Text(product.name)
        Text(product.formattedPrice)
    }
}

// Live region for dynamic content
@Composable
fun Counter() {
    var count by remember { mutableStateOf(0) }

    Column {
        Text(
            text = "Count: $count",
            modifier = Modifier.semantics {
                liveRegion = LiveRegionMode.Polite
            }
        )
        Button(onClick = { count++ }) {
            Text("Increment")
        }
    }
}

// Heading levels
@Composable
fun SectionHeader(title: String, level: Int) {
    Text(
        text = title,
        style = MaterialTheme.typography.headlineMedium,
        modifier = Modifier.semantics {
            heading()
            // Custom heading level (not built-in)
            testTag = "heading-$level"
        }
    )
}
```

### Android XML Views

```xml
<!-- Accessible button -->
<Button
    android:id="@+id/submit_button"
    android:layout_width="wrap_content"
    android:layout_height="48dp"
    android:minWidth="48dp"
    android:text="@string/submit"
    android:contentDescription="@string/submit_form" />

<!-- Grouped content -->
<LinearLayout
    android:layout_width="match_parent"
    android:layout_height="wrap_content"
    android:importantForAccessibility="yes"
    android:focusable="true"
    android:contentDescription="@string/product_description">

    <ImageView
        android:importantForAccessibility="no"
        android:src="@drawable/product" />

    <TextView
        android:text="@string/product_name"
        android:importantForAccessibility="no" />
</LinearLayout>

<!-- Live region -->
<TextView
    android:id="@+id/status"
    android:accessibilityLiveRegion="polite" />
```

```kotlin
// Kotlin accessibility
binding.submitButton.apply {
    contentDescription = getString(R.string.submit_form)
    accessibilityDelegate = object : View.AccessibilityDelegate() {
        override fun onInitializeAccessibilityNodeInfo(
            host: View,
            info: AccessibilityNodeInfo
        ) {
            super.onInitializeAccessibilityNodeInfo(host, info)
            info.addAction(
                AccessibilityNodeInfo.AccessibilityAction(
                    AccessibilityNodeInfo.ACTION_CLICK,
                    getString(R.string.submit_action)
                )
            )
        }
    }
}

// Announce changes
binding.counter.announceForAccessibility("Count updated to $count")
```

## Gesture Accessibility

### Alternative Gestures

```tsx
// React Native: Provide alternatives to complex gestures
function SwipeableCard({ item, onDelete }) {
  const [showDelete, setShowDelete] = useState(false);

  return (
    <View
      accessible={true}
      accessibilityActions={[{ name: "delete", label: "Delete item" }]}
      onAccessibilityAction={(event) => {
        if (event.nativeEvent.actionName === "delete") {
          onDelete(item);
        }
      }}
    >
      <Swipeable
        renderRightActions={() => (
          <TouchableOpacity
            onPress={() => onDelete(item)}
            accessibilityLabel="Delete"
          >
            <Text>Delete</Text>
          </TouchableOpacity>
        )}
      >
        <Text>{item.title}</Text>
      </Swipeable>

      {/* Alternative for screen reader users */}
      <TouchableOpacity
        accessibilityLabel={`Delete ${item.title}`}
        onPress={() => onDelete(item)}
        style={{ position: "absolute", right: 0 }}
      >
        <Text>Delete</Text>
      </TouchableOpacity>
    </View>
  );
}
```

### Motion and Animation

```tsx
// Respect reduced motion preference
import { AccessibilityInfo } from "react-native";

function AnimatedComponent() {
  const [reduceMotion, setReduceMotion] = useState(false);

  useEffect(() => {
    AccessibilityInfo.isReduceMotionEnabled().then(setReduceMotion);

    const subscription = AccessibilityInfo.addEventListener(
      "reduceMotionChanged",
      setReduceMotion,
    );

    return () => subscription.remove();
  }, []);

  return (
    <Animated.View
      style={{
        transform: reduceMotion ? [] : [{ translateX: animatedValue }],
        opacity: reduceMotion ? 1 : animatedOpacity,
      }}
    >
      <Content />
    </Animated.View>
  );
}
```

## Dynamic Type / Text Scaling

### iOS Dynamic Type

```swift
// SwiftUI
Text("Hello, World!")
    .font(.body) // Automatically scales with Dynamic Type

Text("Fixed Size")
    .font(.system(size: 16, design: .default))
    .dynamicTypeSize(.large) // Cap at large

// Allow unlimited scaling
Text("Scalable")
    .font(.body)
    .minimumScaleFactor(0.5)
    .lineLimit(nil)
```

### Android Text Scaling

```xml
<!-- Use sp for text sizes -->
<TextView
    android:textSize="16sp"
    android:layout_width="wrap_content"
    android:layout_height="wrap_content" />

<!-- In styles.xml -->
<style name="TextAppearance.Body">
    <item name="android:textSize">16sp</item>
    <item name="android:lineHeight">24sp</item>
</style>
```

```kotlin
// Compose: Text automatically scales
Text(
    text = "Hello, World!",
    style = MaterialTheme.typography.bodyLarge
)

// Limit scaling if needed
Text(
    text = "Limited scaling",
    fontSize = 16.sp,
    maxLines = 2,
    overflow = TextOverflow.Ellipsis
)
```

### React Native Text Scaling

```tsx
import { Text, PixelRatio } from 'react-native';

// Allow text scaling (default)
<Text allowFontScaling={true}>Scalable text</Text>

// Limit maximum scale
<Text maxFontSizeMultiplier={1.5}>Limited scaling</Text>

// Disable scaling (use sparingly)
<Text allowFontScaling={false}>Fixed size</Text>

// Responsive font size
const scaledFontSize = (size: number) => {
  const scale = PixelRatio.getFontScale();
  return size * Math.min(scale, 1.5); // Cap at 1.5x
};
```

## Testing Checklist

```markdown
## VoiceOver (iOS) Testing

- [ ] All interactive elements have labels
- [ ] Swipe navigation covers all content in logical order
- [ ] Custom actions available for complex interactions
- [ ] Announcements made for dynamic content
- [ ] Headings navigable via rotor
- [ ] Images have appropriate descriptions or are hidden

## TalkBack (Android) Testing

- [ ] Focus order is logical
- [ ] Touch exploration works correctly
- [ ] Custom actions available
- [ ] Live regions announce updates
- [ ] Headings properly marked
- [ ] Grouped content read together

## Motor Accessibility

- [ ] Touch targets at least 44x44 points
- [ ] Adequate spacing between targets (8dp minimum)
- [ ] Alternatives to complex gestures
- [ ] No time-limited interactions

## Visual Accessibility

- [ ] Text scales to 200% without loss
- [ ] Content visible in high contrast mode
- [ ] Color not sole indicator
- [ ] Animations respect reduced motion
```

## Resources

- [Apple Accessibility Programming Guide](https://developer.apple.com/accessibility/)
- [Android Accessibility Developer Guide](https://developer.android.com/guide/topics/ui/accessibility)
- [React Native Accessibility](https://reactnative.dev/docs/accessibility)
- [Mobile Accessibility WCAG](https://www.w3.org/TR/mobile-accessibility-mapping/)


---

## อ้างอิง: wcag-guidelines

# WCAG 2.2 Guidelines Reference

## Overview

The Web Content Accessibility Guidelines (WCAG) 2.2 provide recommendations for making web content more accessible. They are organized into four principles (POUR): Perceivable, Operable, Understandable, and Robust.

## Conformance Levels

- **Level A**: Minimum accessibility (must satisfy)
- **Level AA**: Standard accessibility (should satisfy)
- **Level AAA**: Enhanced accessibility (may satisfy)

Most organizations target Level AA compliance.

## Principle 1: Perceivable

Content must be presentable in ways users can perceive.

### 1.1 Text Alternatives

#### 1.1.1 Non-text Content (Level A)

All non-text content needs text alternatives.

```tsx
// Images
<img src="chart.png" alt="Q3 sales increased 25% compared to Q2" />

// Decorative images
<img src="decorative-line.svg" alt="" role="presentation" />

// Complex images with long descriptions
<figure>
  <img src="org-chart.png" alt="Organization chart" aria-describedby="org-desc" />
  <figcaption id="org-desc">
    The CEO reports to the board. Three VPs report to the CEO:
    VP Engineering, VP Sales, and VP Marketing...
  </figcaption>
</figure>

// Icons with meaning
<button aria-label="Delete item">
  <TrashIcon aria-hidden="true" />
</button>

// Icon buttons with visible text
<button>
  <DownloadIcon aria-hidden="true" />
  <span>Download</span>
</button>
```

### 1.2 Time-based Media

#### 1.2.1 Audio-only and Video-only (Level A)

```tsx
// Audio with transcript
<audio src="podcast.mp3" controls />
<details>
  <summary>View transcript</summary>
  <p>Full transcript text here...</p>
</details>

// Video with captions
<video controls>
  <source src="tutorial.mp4" type="video/mp4" />
  <track kind="captions" src="captions-en.vtt" srclang="en" label="English" />
  <track kind="subtitles" src="subtitles-es.vtt" srclang="es" label="Spanish" />
</video>
```

### 1.3 Adaptable

#### 1.3.1 Info and Relationships (Level A)

Structure and relationships must be programmatically determinable.

```tsx
// Proper heading hierarchy
<main>
  <h1>Page Title</h1>
  <section>
    <h2>Section Title</h2>
    <h3>Subsection</h3>
  </section>
</main>

// Data tables with headers
<table>
  <caption>Quarterly Sales Report</caption>
  <thead>
    <tr>
      <th scope="col">Product</th>
      <th scope="col">Q1</th>
      <th scope="col">Q2</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <th scope="row">Widget A</th>
      <td>$10,000</td>
      <td>$12,000</td>
    </tr>
  </tbody>
</table>

// Lists for grouped content
<nav aria-label="Main navigation">
  <ul>
    <li><a href="/">Home</a></li>
    <li><a href="/about">About</a></li>
    <li><a href="/contact">Contact</a></li>
  </ul>
</nav>
```

#### 1.3.5 Identify Input Purpose (Level AA)

```tsx
// Input with autocomplete for autofill
<form>
  <label htmlFor="name">Full Name</label>
  <input id="name" name="name" autoComplete="name" />

  <label htmlFor="email">Email</label>
  <input id="email" name="email" type="email" autoComplete="email" />

  <label htmlFor="phone">Phone</label>
  <input id="phone" name="phone" type="tel" autoComplete="tel" />

  <label htmlFor="address">Street Address</label>
  <input id="address" name="address" autoComplete="street-address" />

  <label htmlFor="cc">Credit Card Number</label>
  <input id="cc" name="cc" autoComplete="cc-number" />
</form>
```

### 1.4 Distinguishable

#### 1.4.1 Use of Color (Level A)

```tsx
// Bad: Color only indicates error
<input className={hasError ? 'border-red-500' : ''} />

// Good: Color plus icon and text
<div>
  <input
    className={hasError ? 'border-red-500' : ''}
    aria-invalid={hasError}
    aria-describedby={hasError ? 'error-message' : undefined}
  />
  {hasError && (
    <p id="error-message" className="text-red-500 flex items-center gap-1">
      <AlertIcon aria-hidden="true" />
      This field is required
    </p>
  )}
</div>
```

#### 1.4.3 Contrast (Minimum) (Level AA)

```css
/* Minimum contrast ratios */
/* Normal text: 4.5:1 */
/* Large text (18pt+ or 14pt bold+): 3:1 */

/* Good contrast examples */
.text-on-white {
  color: #595959; /* 7:1 ratio on white */
}

.text-on-dark {
  color: #ffffff;
  background: #333333; /* 12.6:1 ratio */
}

/* Link must be distinguishable from surrounding text */
.link {
  color: #0066cc; /* 4.5:1 on white */
  text-decoration: underline; /* Additional visual cue */
}
```

#### 1.4.11 Non-text Contrast (Level AA)

```css
/* UI components need 3:1 contrast */
.button {
  border: 2px solid #767676; /* 3:1 against white */
  background: white;
}

.input {
  border: 1px solid #767676;
}

.input:focus {
  outline: 2px solid #0066cc; /* Focus indicator needs 3:1 */
  outline-offset: 2px;
}

/* Custom checkbox */
.checkbox {
  border: 2px solid #767676;
}

.checkbox:checked {
  background: #0066cc;
  border-color: #0066cc;
}
```

#### 1.4.12 Text Spacing (Level AA)

Content must not be lost when user adjusts text spacing.

```css
/* Allow text spacing adjustments without breaking layout */
.content {
  /* Use relative units */
  line-height: 1.5; /* At least 1.5x font size */
  letter-spacing: 0.12em; /* Support for 0.12em */
  word-spacing: 0.16em; /* Support for 0.16em */

  /* Don't use fixed heights on text containers */
  min-height: auto;

  /* Allow wrapping */
  overflow-wrap: break-word;
}

/* Test with these values: */
/* Line height: 1.5x font size */
/* Letter spacing: 0.12em */
/* Word spacing: 0.16em */
/* Paragraph spacing: 2x font size */
```

#### 1.4.13 Content on Hover or Focus (Level AA)

```tsx
// Tooltip pattern
function Tooltip({ content, children }) {
  const [isVisible, setIsVisible] = useState(false);

  return (
    <div
      onMouseEnter={() => setIsVisible(true)}
      onMouseLeave={() => setIsVisible(false)}
      onFocus={() => setIsVisible(true)}
      onBlur={() => setIsVisible(false)}
    >
      {children}
      {isVisible && (
        <div
          role="tooltip"
          // Dismissible: user can close without moving pointer
          onKeyDown={(e) => e.key === "Escape" && setIsVisible(false)}
          // Hoverable: content stays visible when pointer moves to it
          onMouseEnter={() => setIsVisible(true)}
          onMouseLeave={() => setIsVisible(false)}
          // Persistent: stays until trigger loses focus/hover
        >
          {content}
        </div>
      )}
    </div>
  );
}
```

## Principle 2: Operable

Interface components must be operable by all users.

### 2.1 Keyboard Accessible

#### 2.1.1 Keyboard (Level A)

All functionality must be operable via keyboard.

```tsx
// Custom interactive element
function CustomButton({ onClick, children }) {
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick();
        }
      }}
    >
      {children}
    </div>
  );
}

// Better: just use a button
function BetterButton({ onClick, children }) {
  return <button onClick={onClick}>{children}</button>;
}
```

#### 2.1.2 No Keyboard Trap (Level A)

```tsx
// Modal with proper focus management
function Modal({ isOpen, onClose, children }) {
  const closeButtonRef = useRef(null);

  // Return focus on close
  useEffect(() => {
    if (!isOpen) return;

    const previousFocus = document.activeElement;
    closeButtonRef.current?.focus();

    return () => {
      (previousFocus as HTMLElement)?.focus();
    };
  }, [isOpen]);

  // Allow Escape to close
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <FocusTrap>
      <div role="dialog" aria-modal="true">
        <button ref={closeButtonRef} onClick={onClose}>
          Close
        </button>
        {children}
      </div>
    </FocusTrap>
  );
}
```

### 2.4 Navigable

#### 2.4.1 Bypass Blocks (Level A)

```tsx
// Skip links
<body>
  <a href="#main" className="skip-link">
    Skip to main content
  </a>
  <a href="#nav" className="skip-link">
    Skip to navigation
  </a>

  <header>...</header>

  <nav id="nav" aria-label="Main">
    ...
  </nav>

  <main id="main" tabIndex={-1}>
    {/* Main content */}
  </main>
</body>
```

#### 2.4.4 Link Purpose (In Context) (Level A)

```tsx
// Bad: Ambiguous link text
<a href="/report">Click here</a>
<a href="/report">Read more</a>

// Good: Descriptive link text
<a href="/report">View quarterly sales report</a>

// Good: Context provides meaning
<article>
  <h2>Quarterly Sales Report</h2>
  <p>Sales increased by 25% this quarter...</p>
  <a href="/report">Read full report</a>
</article>

// Good: Visually hidden text for context
<a href="/report">
  Read more
  <span className="sr-only"> about quarterly sales report</span>
</a>
```

#### 2.4.7 Focus Visible (Level AA)

```css
/* Always show focus indicator */
:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}

/* Custom focus styles */
.button:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--color-focus);
}

/* High visibility focus for links */
.link:focus-visible {
  outline: 3px solid var(--color-focus);
  outline-offset: 2px;
  background: var(--color-focus-bg);
}
```

### 2.5 Input Modalities (New in 2.2)

#### 2.5.8 Target Size (Minimum) (Level AA) - NEW

Interactive targets must be at least 24x24 CSS pixels.

```css
/* Minimum target size */
.interactive {
  min-width: 24px;
  min-height: 24px;
}

/* Recommended size for touch (44x44) */
.touch-target {
  min-width: 44px;
  min-height: 44px;
}

/* Inline links are exempt if they have adequate spacing */
.link {
  /* Inline text links don't need minimum size */
  /* but should have adequate line-height */
  line-height: 1.5;
}
```

## Principle 3: Understandable

Content and interface must be understandable.

### 3.1 Readable

#### 3.1.1 Language of Page (Level A)

```html
<!DOCTYPE html>
<html lang="en">
  <head>
    ...
  </head>
  <body>
    ...
  </body>
</html>
```

#### 3.1.2 Language of Parts (Level AA)

```tsx
<p>
  The French phrase <span lang="fr">c'est la vie</span> means "that's life."
</p>
```

### 3.2 Predictable

#### 3.2.2 On Input (Level A)

Don't automatically change context on input.

```tsx
// Bad: Auto-submit on selection
<select onChange={(e) => form.submit()}>
  <option>Select country</option>
</select>

// Good: Explicit submit action
<select onChange={(e) => setCountry(e.target.value)}>
  <option>Select country</option>
</select>
<button type="submit">Continue</button>
```

### 3.3 Input Assistance

#### 3.3.1 Error Identification (Level A)

```tsx
function FormField({ id, label, error, ...props }) {
  return (
    <div>
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        aria-invalid={!!error}
        aria-describedby={error ? `${id}-error` : undefined}
        {...props}
      />
      {error && (
        <p id={`${id}-error`} role="alert" className="text-red-600">
          {error}
        </p>
      )}
    </div>
  );
}
```

#### 3.3.7 Redundant Entry (Level A) - NEW

Don't require users to re-enter previously provided information.

```tsx
// Auto-fill shipping address from billing
function CheckoutForm() {
  const [sameAsBilling, setSameAsBilling] = useState(false);
  const [billing, setBilling] = useState({});
  const [shipping, setShipping] = useState({});

  return (
    <form>
      <fieldset>
        <legend>Billing Address</legend>
        <AddressFields value={billing} onChange={setBilling} />
      </fieldset>

      <label>
        <input
          type="checkbox"
          checked={sameAsBilling}
          onChange={(e) => {
            setSameAsBilling(e.target.checked);
            if (e.target.checked) setShipping(billing);
          }}
        />
        Shipping same as billing
      </label>

      {!sameAsBilling && (
        <fieldset>
          <legend>Shipping Address</legend>
          <AddressFields value={shipping} onChange={setShipping} />
        </fieldset>
      )}
    </form>
  );
}
```

## Principle 4: Robust

Content must be robust enough for assistive technologies.

### 4.1 Compatible

#### 4.1.2 Name, Role, Value (Level A)

```tsx
// Custom components must expose name, role, and value
function CustomCheckbox({ checked, onChange, label }) {
  return (
    <button
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
    >
      {checked ? "✓" : "○"} {label}
    </button>
  );
}

// Custom slider
function CustomSlider({ value, min, max, label, onChange }) {
  return (
    <div
      role="slider"
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={value}
      aria-label={label}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === "ArrowRight") onChange(Math.min(value + 1, max));
        if (e.key === "ArrowLeft") onChange(Math.max(value - 1, min));
      }}
    >
      <div style={{ width: `${((value - min) / (max - min)) * 100}%` }} />
    </div>
  );
}
```

## Testing Checklist

```markdown
## Keyboard Testing

- [ ] All interactive elements focusable with Tab
- [ ] Focus order matches visual order
- [ ] Focus indicator always visible
- [ ] No keyboard traps
- [ ] Escape closes modals/dropdowns
- [ ] Enter/Space activates buttons and links

## Screen Reader Testing

- [ ] All images have alt text
- [ ] Form inputs have labels
- [ ] Headings in logical order
- [ ] Landmarks present (main, nav, header, footer)
- [ ] Dynamic content announced
- [ ] Error messages announced

## Visual Testing

- [ ] Text contrast at least 4.5:1
- [ ] UI component contrast at least 3:1
- [ ] Works at 200% zoom
- [ ] Content readable with text spacing
- [ ] Focus indicators visible
- [ ] Color not sole indicator of meaning
```

## Resources

- [WCAG 2.2 Quick Reference](https://www.w3.org/WAI/WCAG22/quickref/)
- [Understanding WCAG 2.2](https://www.w3.org/WAI/WCAG22/Understanding/)
- [Techniques for WCAG 2.2](https://www.w3.org/WAI/WCAG22/Techniques/)
