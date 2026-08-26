
# Web Component Design

Build reusable, maintainable UI components using modern frameworks with clean composition patterns and styling approaches.

## When to Use This Skill

- Designing reusable component libraries or design systems
- Implementing complex component composition patterns
- Choosing and applying CSS-in-JS solutions
- Building accessible, responsive UI components
- Creating consistent component APIs across a codebase
- Refactoring legacy components into modern patterns
- Implementing compound components or render props

## Core Concepts

### 1. Component Composition Patterns

**Compound Components**: Related components that work together

```tsx
// Usage
<Select value={value} onChange={setValue}>
  <Select.Trigger>Choose option</Select.Trigger>
  <Select.Options>
    <Select.Option value="a">Option A</Select.Option>
    <Select.Option value="b">Option B</Select.Option>
  </Select.Options>
</Select>
```

**Render Props**: Delegate rendering to parent

```tsx
<DataFetcher url="/api/users">
  {({ data, loading, error }) =>
    loading ? <Spinner /> : <UserList users={data} />
  }
</DataFetcher>
```

**Slots (Vue/Svelte)**: Named content injection points

```vue
<template>
  <Card>
    <template #header>Title</template>
    <template #content>Body text</template>
    <template #footer><Button>Action</Button></template>
  </Card>
</template>
```

### 2. CSS-in-JS Approaches

| Solution              | Approach               | Best For                          |
| --------------------- | ---------------------- | --------------------------------- |
| **Tailwind CSS**      | Utility classes        | Rapid prototyping, design systems |
| **CSS Modules**       | Scoped CSS files       | Existing CSS, gradual adoption    |
| **styled-components** | Template literals      | React, dynamic styling            |
| **Emotion**           | Object/template styles | Flexible, SSR-friendly            |
| **Vanilla Extract**   | Zero-runtime           | Performance-critical apps         |

### 3. Component API Design

```tsx
interface ButtonProps {
  variant?: "primary" | "secondary" | "ghost";
  size?: "sm" | "md" | "lg";
  isLoading?: boolean;
  isDisabled?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  children: React.ReactNode;
  onClick?: () => void;
}
```

**Principles**:

- Use semantic prop names (`isLoading` vs `loading`)
- Provide sensible defaults
- Support composition via `children`
- Allow style overrides via `className` or `style`

## Quick Start: React Component with Tailwind

```tsx
import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        primary: "bg-blue-600 text-white hover:bg-blue-700",
        secondary: "bg-gray-100 text-gray-900 hover:bg-gray-200",
        ghost: "hover:bg-gray-100 hover:text-gray-900",
      },
      size: {
        sm: "h-8 px-3 text-sm",
        md: "h-10 px-4 text-sm",
        lg: "h-12 px-6 text-base",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "md",
    },
  },
);

interface ButtonProps
  extends
    ComponentPropsWithoutRef<"button">,
    VariantProps<typeof buttonVariants> {
  isLoading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, isLoading, children, ...props }, ref) => (
    <button
      ref={ref}
      className={cn(buttonVariants({ variant, size }), className)}
      disabled={isLoading || props.disabled}
      {...props}
    >
      {isLoading && <Spinner className="mr-2 h-4 w-4" />}
      {children}
    </button>
  ),
);
Button.displayName = "Button";
```

## Framework Patterns

### React: Compound Components

```tsx
import { createContext, useContext, useState, type ReactNode } from "react";

interface AccordionContextValue {
  openItems: Set<string>;
  toggle: (id: string) => void;
}

const AccordionContext = createContext<AccordionContextValue | null>(null);

function useAccordion() {
  const context = useContext(AccordionContext);
  if (!context) throw new Error("Must be used within Accordion");
  return context;
}

export function Accordion({ children }: { children: ReactNode }) {
  const [openItems, setOpenItems] = useState<Set<string>>(new Set());

  const toggle = (id: string) => {
    setOpenItems((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };

  return (
    <AccordionContext.Provider value={{ openItems, toggle }}>
      <div className="divide-y">{children}</div>
    </AccordionContext.Provider>
  );
}

Accordion.Item = function AccordionItem({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: ReactNode;
}) {
  const { openItems, toggle } = useAccordion();
  const isOpen = openItems.has(id);

  return (
    <div>
      <button onClick={() => toggle(id)} className="w-full text-left py-3">
        {title}
      </button>
      {isOpen && <div className="pb-3">{children}</div>}
    </div>
  );
};
```

### Vue 3: Composables

```vue
<script setup lang="ts">
import { ref, computed, provide, inject, type InjectionKey } from "vue";

interface TabsContext {
  activeTab: Ref<string>;
  setActive: (id: string) => void;
}

const TabsKey: InjectionKey<TabsContext> = Symbol("tabs");

// Parent component
const activeTab = ref("tab-1");
provide(TabsKey, {
  activeTab,
  setActive: (id: string) => {
    activeTab.value = id;
  },
});

// Child component usage
const tabs = inject(TabsKey);
const isActive = computed(() => tabs?.activeTab.value === props.id);
</script>
```

### Svelte 5: Runes

```svelte
<script lang="ts">
  interface Props {
    variant?: 'primary' | 'secondary';
    size?: 'sm' | 'md' | 'lg';
    onclick?: () => void;
    children: import('svelte').Snippet;
  }

  let { variant = 'primary', size = 'md', onclick, children }: Props = $props();

  const classes = $derived(
    `btn btn-${variant} btn-${size}`
  );
</script>

<button class={classes} {onclick}>
  {@render children()}
</button>
```

## Best Practices

1. **Single Responsibility**: Each component does one thing well
2. **Prop Drilling Prevention**: Use context for deeply nested data
3. **Accessible by Default**: Include ARIA attributes, keyboard support
4. **Controlled vs Uncontrolled**: Support both patterns when appropriate
5. **Forward Refs**: Allow parent access to DOM nodes
6. **Memoization**: Use `React.memo`, `useMemo` for expensive renders
7. **Error Boundaries**: Wrap components that may fail

## Common Issues

- **Prop Explosion**: Too many props - consider composition instead
- **Style Conflicts**: Use scoped styles or CSS Modules
- **Re-render Cascades**: Profile with React DevTools, memo appropriately
- **Accessibility Gaps**: Test with screen readers and keyboard navigation
- **Bundle Size**: Tree-shake unused component variants


---

## อ้างอิง: accessibility-patterns

# Accessibility Patterns Reference

## ARIA Patterns for Common Components

### Modal Dialog

```tsx
import { useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";

interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
}

export function Modal({ isOpen, onClose, title, children }: ModalProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const previousActiveElement = useRef<Element | null>(null);

  useEffect(() => {
    if (isOpen) {
      previousActiveElement.current = document.activeElement;
      dialogRef.current?.focus();
      document.body.style.overflow = "hidden";
    } else {
      document.body.style.overflow = "";
      (previousActiveElement.current as HTMLElement)?.focus();
    }

    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "Tab") trapFocus(e, dialogRef.current);
    };

    if (isOpen) {
      document.addEventListener("keydown", handleKeyDown);
    }
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      aria-hidden={!isOpen}
    >
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/50"
        onClick={onClose}
        aria-hidden="true"
      />

      {/* Dialog */}
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        tabIndex={-1}
        className="relative z-10 w-full max-w-md rounded-lg bg-white p-6 shadow-xl"
      >
        <h2 id="modal-title" className="text-lg font-semibold">
          {title}
        </h2>
        <button
          onClick={onClose}
          aria-label="Close dialog"
          className="absolute right-4 top-4 p-1"
        >
          <XIcon aria-hidden="true" />
        </button>
        <div className="mt-4">{children}</div>
      </div>
    </div>,
    document.body,
  );
}

function trapFocus(e: KeyboardEvent, container: HTMLElement | null) {
  if (!container) return;

  const focusableElements = container.querySelectorAll<HTMLElement>(
    'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
  );
  const firstElement = focusableElements[0];
  const lastElement = focusableElements[focusableElements.length - 1];

  if (e.shiftKey && document.activeElement === firstElement) {
    e.preventDefault();
    lastElement.focus();
  } else if (!e.shiftKey && document.activeElement === lastElement) {
    e.preventDefault();
    firstElement.focus();
  }
}
```

### Dropdown Menu

```tsx
import { useState, useRef, useEffect, type ReactNode } from "react";

interface DropdownProps {
  trigger: ReactNode;
  children: ReactNode;
  label: string;
}

export function Dropdown({ trigger, children, label }: DropdownProps) {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (
        containerRef.current &&
        !containerRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false);
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case "Escape":
        setIsOpen(false);
        triggerRef.current?.focus();
        break;
      case "ArrowDown":
        e.preventDefault();
        if (!isOpen) {
          setIsOpen(true);
        } else {
          focusNextItem(menuRef.current, 1);
        }
        break;
      case "ArrowUp":
        e.preventDefault();
        if (isOpen) {
          focusNextItem(menuRef.current, -1);
        }
        break;
      case "Home":
        e.preventDefault();
        focusFirstItem(menuRef.current);
        break;
      case "End":
        e.preventDefault();
        focusLastItem(menuRef.current);
        break;
    }
  };

  return (
    <div ref={containerRef} className="relative" onKeyDown={handleKeyDown}>
      <button
        ref={triggerRef}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        aria-label={label}
        onClick={() => setIsOpen(!isOpen)}
        className="flex items-center gap-2 px-3 py-2"
      >
        {trigger}
        <ChevronDownIcon
          aria-hidden="true"
          className={`transition-transform ${isOpen ? "rotate-180" : ""}`}
        />
      </button>

      {isOpen && (
        <div
          ref={menuRef}
          role="menu"
          aria-orientation="vertical"
          className="absolute left-0 mt-1 min-w-48 rounded-md bg-white py-1 shadow-lg ring-1 ring-black/5"
        >
          {children}
        </div>
      )}
    </div>
  );
}

interface MenuItemProps {
  children: ReactNode;
  onClick?: () => void;
  disabled?: boolean;
}

export function MenuItem({ children, onClick, disabled }: MenuItemProps) {
  return (
    <button
      role="menuitem"
      disabled={disabled}
      onClick={onClick}
      className="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 disabled:opacity-50"
      tabIndex={-1}
    >
      {children}
    </button>
  );
}

function focusNextItem(menu: HTMLElement | null, direction: 1 | -1) {
  if (!menu) return;
  const items = menu.querySelectorAll<HTMLElement>(
    '[role="menuitem"]:not([disabled])',
  );
  const currentIndex = Array.from(items).indexOf(
    document.activeElement as HTMLElement,
  );
  const nextIndex = (currentIndex + direction + items.length) % items.length;
  items[nextIndex]?.focus();
}

function focusFirstItem(menu: HTMLElement | null) {
  menu
    ?.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])')
    ?.focus();
}

function focusLastItem(menu: HTMLElement | null) {
  const items = menu?.querySelectorAll<HTMLElement>(
    '[role="menuitem"]:not([disabled])',
  );
  items?.[items.length - 1]?.focus();
}
```

### Combobox / Autocomplete

```tsx
import {
  useState,
  useRef,
  useId,
  type ChangeEvent,
  type KeyboardEvent,
} from "react";

interface Option {
  value: string;
  label: string;
}

interface ComboboxProps {
  options: Option[];
  value: string;
  onChange: (value: string) => void;
  label: string;
  placeholder?: string;
}

export function Combobox({
  options,
  value,
  onChange,
  label,
  placeholder,
}: ComboboxProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [inputValue, setInputValue] = useState("");
  const [activeIndex, setActiveIndex] = useState(-1);

  const inputRef = useRef<HTMLInputElement>(null);
  const listboxRef = useRef<HTMLUListElement>(null);
  const inputId = useId();
  const listboxId = useId();

  const filteredOptions = options.filter((option) =>
    option.label.toLowerCase().includes(inputValue.toLowerCase()),
  );

  const handleInputChange = (e: ChangeEvent<HTMLInputElement>) => {
    setInputValue(e.target.value);
    setIsOpen(true);
    setActiveIndex(-1);
  };

  const handleSelect = (option: Option) => {
    onChange(option.value);
    setInputValue(option.label);
    setIsOpen(false);
    inputRef.current?.focus();
  };

  const handleKeyDown = (e: KeyboardEvent) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        if (!isOpen) {
          setIsOpen(true);
        } else {
          setActiveIndex((prev) =>
            prev < filteredOptions.length - 1 ? prev + 1 : prev,
          );
        }
        break;
      case "ArrowUp":
        e.preventDefault();
        setActiveIndex((prev) => (prev > 0 ? prev - 1 : prev));
        break;
      case "Enter":
        e.preventDefault();
        if (activeIndex >= 0 && filteredOptions[activeIndex]) {
          handleSelect(filteredOptions[activeIndex]);
        }
        break;
      case "Escape":
        setIsOpen(false);
        break;
    }
  };

  return (
    <div className="relative">
      <label htmlFor={inputId} className="block text-sm font-medium mb-1">
        {label}
      </label>
      <input
        ref={inputRef}
        id={inputId}
        type="text"
        role="combobox"
        aria-expanded={isOpen}
        aria-autocomplete="list"
        aria-controls={listboxId}
        aria-activedescendant={
          activeIndex >= 0 ? `option-${activeIndex}` : undefined
        }
        value={inputValue}
        placeholder={placeholder}
        onChange={handleInputChange}
        onKeyDown={handleKeyDown}
        onFocus={() => setIsOpen(true)}
        onBlur={() => setTimeout(() => setIsOpen(false), 200)}
        className="w-full rounded-md border px-3 py-2"
      />

      {isOpen && filteredOptions.length > 0 && (
        <ul
          ref={listboxRef}
          id={listboxId}
          role="listbox"
          aria-label={label}
          className="absolute z-10 mt-1 max-h-60 w-full overflow-auto rounded-md bg-white py-1 shadow-lg ring-1 ring-black/5"
        >
          {filteredOptions.map((option, index) => (
            <li
              key={option.value}
              id={`option-${index}`}
              role="option"
              aria-selected={activeIndex === index}
              onClick={() => handleSelect(option)}
              className={`cursor-pointer px-3 py-2 ${
                activeIndex === index ? "bg-blue-100" : "hover:bg-gray-100"
              } ${value === option.value ? "font-medium" : ""}`}
            >
              {option.label}
            </li>
          ))}
        </ul>
      )}

      {isOpen && filteredOptions.length === 0 && (
        <div className="absolute z-10 mt-1 w-full rounded-md bg-white px-3 py-2 shadow-lg">
          No results found
        </div>
      )}
    </div>
  );
}
```

### Form Validation

```tsx
import { useId, type FormEvent } from "react";

interface FormFieldProps {
  label: string;
  error?: string;
  required?: boolean;
  children: (props: {
    id: string;
    "aria-describedby": string | undefined;
    "aria-invalid": boolean;
  }) => ReactNode;
}

export function FormField({
  label,
  error,
  required,
  children,
}: FormFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;

  return (
    <div className="space-y-1">
      <label htmlFor={id} className="block text-sm font-medium">
        {label}
        {required && (
          <span aria-hidden="true" className="ml-1 text-red-500">
            *
          </span>
        )}
      </label>

      {children({
        id,
        "aria-describedby": error ? errorId : undefined,
        "aria-invalid": !!error,
      })}

      {error && (
        <p id={errorId} role="alert" className="text-sm text-red-600">
          {error}
        </p>
      )}
    </div>
  );
}

// Usage
function ContactForm() {
  const [errors, setErrors] = useState<Record<string, string>>({});

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    // Validation logic...
  };

  return (
    <form onSubmit={handleSubmit} noValidate>
      <FormField label="Email" error={errors.email} required>
        {(props) => (
          <input
            {...props}
            type="email"
            required
            className={`w-full rounded border px-3 py-2 ${
              props["aria-invalid"] ? "border-red-500" : "border-gray-300"
            }`}
          />
        )}
      </FormField>

      <button
        type="submit"
        className="mt-4 px-4 py-2 bg-blue-600 text-white rounded"
      >
        Submit
      </button>
    </form>
  );
}
```

## Skip Links

```tsx
export function SkipLinks() {
  return (
    <div className="sr-only focus-within:not-sr-only">
      <a
        href="#main-content"
        className="absolute left-4 top-4 z-50 rounded bg-blue-600 px-4 py-2 text-white focus:outline-none focus:ring-2"
      >
        Skip to main content
      </a>
      <a
        href="#main-navigation"
        className="absolute left-4 top-16 z-50 rounded bg-blue-600 px-4 py-2 text-white focus:outline-none focus:ring-2"
      >
        Skip to navigation
      </a>
    </div>
  );
}
```

## Live Regions

```tsx
import { useState, useEffect } from "react";

interface LiveAnnouncerProps {
  message: string;
  politeness?: "polite" | "assertive";
}

export function LiveAnnouncer({
  message,
  politeness = "polite",
}: LiveAnnouncerProps) {
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    // Clear first, then set - ensures screen readers pick up the change
    setAnnouncement("");
    const timer = setTimeout(() => setAnnouncement(message), 100);
    return () => clearTimeout(timer);
  }, [message]);

  return (
    <div
      role="status"
      aria-live={politeness}
      aria-atomic="true"
      className="sr-only"
    >
      {announcement}
    </div>
  );
}

// Usage in a search component
function SearchResults({
  results,
  loading,
}: {
  results: Item[];
  loading: boolean;
}) {
  const message = loading
    ? "Loading results..."
    : `${results.length} results found`;

  return (
    <>
      <LiveAnnouncer message={message} />
      <ul>{/* results */}</ul>
    </>
  );
}
```

## Focus Management Utilities

```tsx
// useFocusReturn - restore focus after closing
function useFocusReturn() {
  const previousElement = useRef<Element | null>(null);

  const saveFocus = () => {
    previousElement.current = document.activeElement;
  };

  const restoreFocus = () => {
    (previousElement.current as HTMLElement)?.focus();
  };

  return { saveFocus, restoreFocus };
}

// useFocusTrap - keep focus within container
function useFocusTrap(containerRef: RefObject<HTMLElement>, isActive: boolean) {
  useEffect(() => {
    if (!isActive || !containerRef.current) return;

    const container = containerRef.current;
    const focusableSelector =
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Tab") return;

      const focusableElements =
        container.querySelectorAll<HTMLElement>(focusableSelector);
      const first = focusableElements[0];
      const last = focusableElements[focusableElements.length - 1];

      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };

    container.addEventListener("keydown", handleKeyDown);
    return () => container.removeEventListener("keydown", handleKeyDown);
  }, [containerRef, isActive]);
}
```

## Color Contrast Utilities

```tsx
// Check if colors meet WCAG requirements
function getContrastRatio(fg: string, bg: string): number {
  const getLuminance = (hex: string): number => {
    const rgb = parseInt(hex.slice(1), 16);
    const r = (rgb >> 16) & 0xff;
    const g = (rgb >> 8) & 0xff;
    const b = rgb & 0xff;

    const [rs, gs, bs] = [r, g, b].map((c) => {
      c = c / 255;
      return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
    });

    return 0.2126 * rs + 0.7152 * gs + 0.0722 * bs;
  };

  const l1 = getLuminance(fg);
  const l2 = getLuminance(bg);
  const lighter = Math.max(l1, l2);
  const darker = Math.min(l1, l2);

  return (lighter + 0.05) / (darker + 0.05);
}

function meetsWCAG(
  fg: string,
  bg: string,
  level: "AA" | "AAA" = "AA",
): boolean {
  const ratio = getContrastRatio(fg, bg);
  return level === "AAA" ? ratio >= 7 : ratio >= 4.5;
}
```


---

## อ้างอิง: component-patterns

# Component Patterns Reference

## Compound Components Deep Dive

Compound components share implicit state while allowing flexible composition.

### Implementation with Context

```tsx
import {
  createContext,
  useContext,
  useState,
  useCallback,
  type ReactNode,
  type Dispatch,
  type SetStateAction,
} from "react";

// Types
interface TabsContextValue {
  activeTab: string;
  setActiveTab: Dispatch<SetStateAction<string>>;
}

interface TabsProps {
  defaultValue: string;
  children: ReactNode;
  onChange?: (value: string) => void;
}

interface TabListProps {
  children: ReactNode;
  className?: string;
}

interface TabProps {
  value: string;
  children: ReactNode;
  disabled?: boolean;
}

interface TabPanelProps {
  value: string;
  children: ReactNode;
}

// Context
const TabsContext = createContext<TabsContextValue | null>(null);

function useTabs() {
  const context = useContext(TabsContext);
  if (!context) {
    throw new Error("Tabs components must be used within <Tabs>");
  }
  return context;
}

// Root Component
export function Tabs({ defaultValue, children, onChange }: TabsProps) {
  const [activeTab, setActiveTab] = useState(defaultValue);

  const handleChange: Dispatch<SetStateAction<string>> = useCallback(
    (value) => {
      const newValue = typeof value === "function" ? value(activeTab) : value;
      setActiveTab(newValue);
      onChange?.(newValue);
    },
    [activeTab, onChange],
  );

  return (
    <TabsContext.Provider value={{ activeTab, setActiveTab: handleChange }}>
      <div className="tabs">{children}</div>
    </TabsContext.Provider>
  );
}

// Tab List (container for tab triggers)
Tabs.List = function TabList({ children, className }: TabListProps) {
  return (
    <div role="tablist" className={`flex border-b ${className}`}>
      {children}
    </div>
  );
};

// Individual Tab (trigger)
Tabs.Tab = function Tab({ value, children, disabled }: TabProps) {
  const { activeTab, setActiveTab } = useTabs();
  const isActive = activeTab === value;

  return (
    <button
      role="tab"
      aria-selected={isActive}
      aria-controls={`panel-${value}`}
      tabIndex={isActive ? 0 : -1}
      disabled={disabled}
      onClick={() => setActiveTab(value)}
      className={`
        px-4 py-2 font-medium transition-colors
        ${
          isActive
            ? "border-b-2 border-blue-600 text-blue-600"
            : "text-gray-600 hover:text-gray-900"
        }
        ${disabled ? "opacity-50 cursor-not-allowed" : ""}
      `}
    >
      {children}
    </button>
  );
};

// Tab Panel (content)
Tabs.Panel = function TabPanel({ value, children }: TabPanelProps) {
  const { activeTab } = useTabs();

  if (activeTab !== value) return null;

  return (
    <div
      role="tabpanel"
      id={`panel-${value}`}
      aria-labelledby={`tab-${value}`}
      tabIndex={0}
      className="py-4"
    >
      {children}
    </div>
  );
};
```

### Usage

```tsx
<Tabs defaultValue="overview" onChange={console.log}>
  <Tabs.List>
    <Tabs.Tab value="overview">Overview</Tabs.Tab>
    <Tabs.Tab value="features">Features</Tabs.Tab>
    <Tabs.Tab value="pricing" disabled>
      Pricing
    </Tabs.Tab>
  </Tabs.List>
  <Tabs.Panel value="overview">
    <h2>Product Overview</h2>
    <p>Description here...</p>
  </Tabs.Panel>
  <Tabs.Panel value="features">
    <h2>Key Features</h2>
    <ul>...</ul>
  </Tabs.Panel>
</Tabs>
```

## Render Props Pattern

Delegate rendering control to the consumer while providing state and helpers.

```tsx
interface DataLoaderRenderProps<T> {
  data: T | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface DataLoaderProps<T> {
  url: string;
  children: (props: DataLoaderRenderProps<T>) => ReactNode;
}

function DataLoader<T>({ url, children }: DataLoaderProps<T>) {
  const [state, setState] = useState<{
    data: T | null;
    loading: boolean;
    error: Error | null;
  }>({
    data: null,
    loading: true,
    error: null,
  });

  const fetchData = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await fetch(url);
      if (!response.ok) throw new Error("Fetch failed");
      const data = await response.json();
      setState({ data, loading: false, error: null });
    } catch (error) {
      setState((prev) => ({ ...prev, loading: false, error: error as Error }));
    }
  }, [url]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return <>{children({ ...state, refetch: fetchData })}</>;
}

// Usage
<DataLoader<User[]> url="/api/users">
  {({ data, loading, error, refetch }) => {
    if (loading) return <Spinner />;
    if (error) return <ErrorMessage error={error} onRetry={refetch} />;
    return <UserList users={data!} />;
  }}
</DataLoader>;
```

## Polymorphic Components

Components that can render as different HTML elements.

```tsx
type AsProp<C extends React.ElementType> = {
  as?: C;
};

type PropsToOmit<C extends React.ElementType, P> = keyof (AsProp<C> & P);

type PolymorphicComponentProp<
  C extends React.ElementType,
  Props = {}
> = React.PropsWithChildren<Props & AsProp<C>> &
  Omit<React.ComponentPropsWithoutRef<C>, PropsToOmit<C, Props>>;

interface TextOwnProps {
  variant?: 'body' | 'heading' | 'label';
  size?: 'sm' | 'md' | 'lg';
}

type TextProps<C extends React.ElementType> = PolymorphicComponentProp<C, TextOwnProps>;

function Text<C extends React.ElementType = 'span'>({
  as,
  variant = 'body',
  size = 'md',
  className,
  children,
  ...props
}: TextProps<C>) {
  const Component = as || 'span';

  const variantClasses = {
    body: 'font-normal',
    heading: 'font-bold',
    label: 'font-medium uppercase tracking-wide',
  };

  const sizeClasses = {
    sm: 'text-sm',
    md: 'text-base',
    lg: 'text-lg',
  };

  return (
    <Component
      className={`${variantClasses[variant]} ${sizeClasses[size]} ${className}`}
      {...props}
    >
      {children}
    </Component>
  );
}

// Usage
<Text>Default span</Text>
<Text as="p" variant="body" size="lg">Paragraph</Text>
<Text as="h1" variant="heading" size="lg">Heading</Text>
<Text as="label" variant="label" htmlFor="input">Label</Text>
```

## Controlled vs Uncontrolled Pattern

Support both modes for maximum flexibility.

```tsx
interface InputProps {
  // Controlled
  value?: string;
  onChange?: (value: string) => void;
  // Uncontrolled
  defaultValue?: string;
  // Common
  placeholder?: string;
  disabled?: boolean;
}

function Input({
  value: controlledValue,
  onChange,
  defaultValue = '',
  ...props
}: InputProps) {
  const [internalValue, setInternalValue] = useState(defaultValue);

  // Determine if controlled
  const isControlled = controlledValue !== undefined;
  const value = isControlled ? controlledValue : internalValue;

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;

    if (!isControlled) {
      setInternalValue(newValue);
    }

    onChange?.(newValue);
  };

  return (
    <input
      type="text"
      value={value}
      onChange={handleChange}
      {...props}
    />
  );
}

// Controlled usage
const [search, setSearch] = useState('');
<Input value={search} onChange={setSearch} />

// Uncontrolled usage
<Input defaultValue="initial" onChange={console.log} />
```

## Slot Pattern

Allow consumers to replace internal parts.

```tsx
interface CardProps {
  children: ReactNode;
  header?: ReactNode;
  footer?: ReactNode;
  media?: ReactNode;
}

function Card({ children, header, footer, media }: CardProps) {
  return (
    <article className="rounded-lg border bg-white shadow-sm">
      {media && (
        <div className="aspect-video overflow-hidden rounded-t-lg">{media}</div>
      )}
      {header && <header className="border-b px-4 py-3">{header}</header>}
      <div className="px-4 py-4">{children}</div>
      {footer && (
        <footer className="border-t px-4 py-3 bg-gray-50 rounded-b-lg">
          {footer}
        </footer>
      )}
    </article>
  );
}

// Usage with slots
<Card
  media={<img src="/image.jpg" alt="" />}
  header={<h2 className="font-semibold">Card Title</h2>}
  footer={<Button>Action</Button>}
>
  <p>Card content goes here.</p>
</Card>;
```

## Forward Ref Pattern

Allow parent components to access the underlying DOM node.

```tsx
import { forwardRef, useRef, useImperativeHandle } from "react";

interface InputHandle {
  focus: () => void;
  clear: () => void;
  getValue: () => string;
}

interface FancyInputProps {
  label: string;
  placeholder?: string;
}

const FancyInput = forwardRef<InputHandle, FancyInputProps>(
  ({ label, placeholder }, ref) => {
    const inputRef = useRef<HTMLInputElement>(null);

    useImperativeHandle(ref, () => ({
      focus: () => inputRef.current?.focus(),
      clear: () => {
        if (inputRef.current) inputRef.current.value = "";
      },
      getValue: () => inputRef.current?.value ?? "",
    }));

    return (
      <div>
        <label className="block text-sm font-medium mb-1">{label}</label>
        <input
          ref={inputRef}
          type="text"
          placeholder={placeholder}
          className="w-full px-3 py-2 border rounded-md"
        />
      </div>
    );
  },
);

FancyInput.displayName = "FancyInput";

// Usage
function Form() {
  const inputRef = useRef<InputHandle>(null);

  const handleSubmit = () => {
    console.log(inputRef.current?.getValue());
    inputRef.current?.clear();
  };

  return (
    <form onSubmit={handleSubmit}>
      <FancyInput ref={inputRef} label="Name" />
      <button type="button" onClick={() => inputRef.current?.focus()}>
        Focus Input
      </button>
    </form>
  );
}
```


---

## อ้างอิง: css-styling-approaches

# CSS Styling Approaches Reference

## Comparison Matrix

| Approach          | Runtime | Bundle Size    | Learning Curve | Dynamic Styles | SSR   |
| ----------------- | ------- | -------------- | -------------- | -------------- | ----- |
| CSS Modules       | None    | Minimal        | Low            | Limited        | Yes   |
| Tailwind          | None    | Small (purged) | Medium         | Via classes    | Yes   |
| styled-components | Yes     | Medium         | Medium         | Full           | Yes\* |
| Emotion           | Yes     | Medium         | Medium         | Full           | Yes   |
| Vanilla Extract   | None    | Minimal        | High           | Limited        | Yes   |

## CSS Modules

Scoped CSS with zero runtime overhead.

### Setup

```tsx
// Button.module.css
.button {
  padding: 0.5rem 1rem;
  border-radius: 0.375rem;
  font-weight: 500;
  transition: background-color 0.2s;
}

.primary {
  background-color: #2563eb;
  color: white;
}

.primary:hover {
  background-color: #1d4ed8;
}

.secondary {
  background-color: #f3f4f6;
  color: #1f2937;
}

.secondary:hover {
  background-color: #e5e7eb;
}

.small {
  padding: 0.25rem 0.5rem;
  font-size: 0.875rem;
}

.large {
  padding: 0.75rem 1.5rem;
  font-size: 1.125rem;
}
```

```tsx
// Button.tsx
import styles from "./Button.module.css";
import { clsx } from "clsx";

interface ButtonProps {
  variant?: "primary" | "secondary";
  size?: "small" | "medium" | "large";
  children: React.ReactNode;
  onClick?: () => void;
}

export function Button({
  variant = "primary",
  size = "medium",
  children,
  onClick,
}: ButtonProps) {
  return (
    <button
      className={clsx(
        styles.button,
        styles[variant],
        size !== "medium" && styles[size],
      )}
      onClick={onClick}
    >
      {children}
    </button>
  );
}
```

### Composition

```css
/* base.module.css */
.visuallyHidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  border: 0;
}

/* Button.module.css */
.srOnly {
  composes: visuallyHidden from "./base.module.css";
}
```

## Tailwind CSS

Utility-first CSS with design system constraints.

### Class Variance Authority (CVA)

```tsx
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  // Base styles
  "inline-flex items-center justify-center rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive/90",
        outline:
          "border border-input bg-background hover:bg-accent hover:text-accent-foreground",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-10 px-4 py-2",
        sm: "h-9 rounded-md px-3",
        lg: "h-11 rounded-md px-8",
        icon: "h-10 w-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

interface ButtonProps
  extends
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return (
      <button
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
```

### Tailwind Merge Utility

```tsx
// lib/utils.ts
import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Usage - handles conflicting classes
cn("px-4 py-2", "px-6"); // => 'py-2 px-6'
cn("text-red-500", condition && "text-blue-500"); // => 'text-blue-500' if condition
```

### Custom Plugin

```js
// tailwind.config.js
const plugin = require("tailwindcss/plugin");

module.exports = {
  plugins: [
    plugin(function ({ addUtilities, addComponents, theme }) {
      // Add utilities
      addUtilities({
        ".text-balance": {
          "text-wrap": "balance",
        },
        ".scrollbar-hide": {
          "-ms-overflow-style": "none",
          "scrollbar-width": "none",
          "&::-webkit-scrollbar": {
            display: "none",
          },
        },
      });

      // Add components
      addComponents({
        ".card": {
          backgroundColor: theme("colors.white"),
          borderRadius: theme("borderRadius.lg"),
          padding: theme("spacing.6"),
          boxShadow: theme("boxShadow.md"),
        },
      });
    }),
  ],
};
```

## styled-components

CSS-in-JS with template literals.

```tsx
import styled, { css, keyframes } from "styled-components";

// Keyframes
const fadeIn = keyframes`
  from { opacity: 0; transform: translateY(-10px); }
  to { opacity: 1; transform: translateY(0); }
`;

// Base button with variants
interface ButtonProps {
  $variant?: "primary" | "secondary" | "ghost";
  $size?: "sm" | "md" | "lg";
  $isLoading?: boolean;
}

const sizeStyles = {
  sm: css`
    padding: 0.25rem 0.75rem;
    font-size: 0.875rem;
  `,
  md: css`
    padding: 0.5rem 1rem;
    font-size: 1rem;
  `,
  lg: css`
    padding: 0.75rem 1.5rem;
    font-size: 1.125rem;
  `,
};

const variantStyles = {
  primary: css`
    background-color: ${({ theme }) => theme.colors.primary};
    color: white;
    &:hover:not(:disabled) {
      background-color: ${({ theme }) => theme.colors.primaryHover};
    }
  `,
  secondary: css`
    background-color: ${({ theme }) => theme.colors.secondary};
    color: ${({ theme }) => theme.colors.text};
    &:hover:not(:disabled) {
      background-color: ${({ theme }) => theme.colors.secondaryHover};
    }
  `,
  ghost: css`
    background-color: transparent;
    color: ${({ theme }) => theme.colors.text};
    &:hover:not(:disabled) {
      background-color: ${({ theme }) => theme.colors.ghost};
    }
  `,
};

const Button = styled.button<ButtonProps>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 0.375rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  animation: ${fadeIn} 0.3s ease;

  ${({ $size = "md" }) => sizeStyles[$size]}
  ${({ $variant = "primary" }) => variantStyles[$variant]}

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  ${({ $isLoading }) =>
    $isLoading &&
    css`
      pointer-events: none;
      opacity: 0.7;
    `}
`;

// Extending components
const IconButton = styled(Button)`
  padding: 0.5rem;
  aspect-ratio: 1;
`;

// Theme provider
const theme = {
  colors: {
    primary: "#2563eb",
    primaryHover: "#1d4ed8",
    secondary: "#f3f4f6",
    secondaryHover: "#e5e7eb",
    ghost: "rgba(0, 0, 0, 0.05)",
    text: "#1f2937",
  },
};

// Usage
<ThemeProvider theme={theme}>
  <Button $variant="primary" $size="lg">
    Click me
  </Button>
</ThemeProvider>;
```

## Emotion

Flexible CSS-in-JS with object and template syntax.

```tsx
/** @jsxImportSource @emotion/react */
import { css, Theme, ThemeProvider, useTheme } from "@emotion/react";
import styled from "@emotion/styled";

// Theme typing
declare module "@emotion/react" {
  export interface Theme {
    colors: {
      primary: string;
      background: string;
      text: string;
    };
    spacing: (factor: number) => string;
  }
}

const theme: Theme = {
  colors: {
    primary: "#2563eb",
    background: "#ffffff",
    text: "#1f2937",
  },
  spacing: (factor: number) => `${factor * 0.25}rem`,
};

// Object syntax
const cardStyles = (theme: Theme) =>
  css({
    backgroundColor: theme.colors.background,
    padding: theme.spacing(4),
    borderRadius: "0.5rem",
    boxShadow: "0 1px 3px rgba(0, 0, 0, 0.1)",
  });

// Template literal syntax
const buttonStyles = css`
  padding: 0.5rem 1rem;
  border-radius: 0.375rem;
  font-weight: 500;

  &:hover {
    opacity: 0.9;
  }
`;

// Styled component with theme
const Card = styled.div`
  background-color: ${({ theme }) => theme.colors.background};
  padding: ${({ theme }) => theme.spacing(4)};
  border-radius: 0.5rem;
`;

// Component with css prop
function Alert({ children }: { children: React.ReactNode }) {
  const theme = useTheme();

  return (
    <div
      css={css`
        padding: ${theme.spacing(3)};
        background-color: ${theme.colors.primary}10;
        border-left: 4px solid ${theme.colors.primary};
      `}
    >
      {children}
    </div>
  );
}

// Usage
<ThemeProvider theme={theme}>
  <Card>
    <Alert>Important message</Alert>
  </Card>
</ThemeProvider>;
```

## Vanilla Extract

Zero-runtime CSS-in-JS with full type safety.

```tsx
// styles.css.ts
import { style, styleVariants, createTheme } from "@vanilla-extract/css";
import { recipe, type RecipeVariants } from "@vanilla-extract/recipes";

// Theme contract
export const [themeClass, vars] = createTheme({
  color: {
    primary: "#2563eb",
    secondary: "#64748b",
    background: "#ffffff",
    text: "#1f2937",
  },
  space: {
    small: "0.5rem",
    medium: "1rem",
    large: "1.5rem",
  },
  radius: {
    small: "0.25rem",
    medium: "0.375rem",
    large: "0.5rem",
  },
});

// Simple style
export const container = style({
  padding: vars.space.medium,
  backgroundColor: vars.color.background,
});

// Style variants
export const text = styleVariants({
  primary: { color: vars.color.text },
  secondary: { color: vars.color.secondary },
  accent: { color: vars.color.primary },
});

// Recipe (like CVA)
export const button = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    fontWeight: 500,
    borderRadius: vars.radius.medium,
    transition: "background-color 0.2s",
    cursor: "pointer",
    border: "none",
    ":disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
  variants: {
    variant: {
      primary: {
        backgroundColor: vars.color.primary,
        color: "white",
        ":hover": {
          backgroundColor: "#1d4ed8",
        },
      },
      secondary: {
        backgroundColor: "#f3f4f6",
        color: vars.color.text,
        ":hover": {
          backgroundColor: "#e5e7eb",
        },
      },
    },
    size: {
      small: {
        padding: "0.25rem 0.75rem",
        fontSize: "0.875rem",
      },
      medium: {
        padding: "0.5rem 1rem",
        fontSize: "1rem",
      },
      large: {
        padding: "0.75rem 1.5rem",
        fontSize: "1.125rem",
      },
    },
  },
  defaultVariants: {
    variant: "primary",
    size: "medium",
  },
});

export type ButtonVariants = RecipeVariants<typeof button>;
```

```tsx
// Button.tsx
import { button, type ButtonVariants, themeClass } from "./styles.css";

interface ButtonProps extends ButtonVariants {
  children: React.ReactNode;
  onClick?: () => void;
}

export function Button({ variant, size, children, onClick }: ButtonProps) {
  return (
    <button className={button({ variant, size })} onClick={onClick}>
      {children}
    </button>
  );
}

// App.tsx - wrap with theme
function App() {
  return (
    <div className={themeClass}>
      <Button variant="primary" size="large">
        Click me
      </Button>
    </div>
  );
}
```

## Performance Considerations

### Critical CSS Extraction

```tsx
// Next.js with styled-components
// pages/_document.tsx
import Document, { DocumentContext } from "next/document";
import { ServerStyleSheet } from "styled-components";

export default class MyDocument extends Document {
  static async getInitialProps(ctx: DocumentContext) {
    const sheet = new ServerStyleSheet();
    const originalRenderPage = ctx.renderPage;

    try {
      ctx.renderPage = () =>
        originalRenderPage({
          enhanceApp: (App) => (props) =>
            sheet.collectStyles(<App {...props} />),
        });

      const initialProps = await Document.getInitialProps(ctx);
      return {
        ...initialProps,
        styles: [initialProps.styles, sheet.getStyleElement()],
      };
    } finally {
      sheet.seal();
    }
  }
}
```

### Code Splitting Styles

```tsx
// Dynamically import heavy styled components
import dynamic from "next/dynamic";

const HeavyChart = dynamic(() => import("./HeavyChart"), {
  loading: () => <Skeleton height={400} />,
  ssr: false,
});
```
