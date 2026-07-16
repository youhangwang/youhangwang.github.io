# frozen_string_literal: true

# Compatibility shim for running Jekyll 4.2.x / Liquid 4.0.x on Ruby >= 3.2.
#
# Ruby 3.2 removed Object#taint, #tainted? and #untaint (the taint mechanism
# was deprecated in 2.7 and removed in 3.2). Liquid 4.0.3 — pinned by
# jekyll 4.2.x — still calls #tainted? in Liquid::Variable#taint_check,
# raising `undefined method 'tainted?' for true` at render time.
#
# Restore the methods as no-ops (their semantics were already no-ops in modern
# Ruby) so old Liquid renders unchanged on Ruby 3.2+. Harmless on older Ruby
# where the methods still exist (the guard skips redefinition).
unless Object.instance_methods.include?(:tainted?)
  class Object
    def tainted?
      false
    end

    def taint
      self
    end

    def untaint
      self
    end
  end
end
